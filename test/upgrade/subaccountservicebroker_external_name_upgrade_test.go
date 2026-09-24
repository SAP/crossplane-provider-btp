//go:build upgrade

package upgrade

import (
	"context"
	"os"
	"strings"
	"testing"

	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	// v1.11.0 is the newest stable release (no -rc suffix). It still writes the bare broker
	// GUID into the external-name annotation, so upgrading from it to the local build exercises
	// the migration to the compound key "<subaccount-id>/<broker-id>".
	brokerFromCustomTag             = "v1.11.0"
	brokerToCustomTag               = "local"
	brokerCustomResourceDirectories = []string{
		upgradeCRsPath("customCRs/subaccountServiceBrokerExternalName"),
	}
)

// ADR(external-name): SubaccountServiceBroker migrates from bare broker GUID to compound key "<subaccount-id>/<broker-id>" on upgrade.
//
// Test_SubaccountServiceBroker_External_Name verifies the migration of the external-name
// annotation during a provider upgrade:
//  1. Before upgrade (old provider) the annotation is the bare broker GUID (no "/").
//  2. After upgrade the annotation is the compound key "<subaccount-id>/<broker-id>".
//  3. The broker id is preserved through the migration and the subaccount id matches
//     the CR's spec.forProvider.subaccountId.
func Test_SubaccountServiceBroker_External_Name(t *testing.T) {
	const brokerName = "upgrade-test-extn-broker"

	// The broker cannot reach Ready without a publicly reachable service broker to register
	// against. Gate on all three variables: the fixture CR templates $SERVICE_BROKER_URL and
	// $SERVICE_BROKER_USERNAME (envsubst), and the credentials Secret needs
	// $SERVICE_BROKER_PASSWORD — a partial configuration must skip, not run and fail.
	if os.Getenv("SERVICE_BROKER_URL") == "" || os.Getenv("SERVICE_BROKER_USERNAME") == "" || os.Getenv("SERVICE_BROKER_PASSWORD") == "" {
		t.Skip("SERVICE_BROKER_URL, SERVICE_BROKER_USERNAME or SERVICE_BROKER_PASSWORD is not set; skipping SubaccountServiceBroker external-name upgrade test (needs a reachable service broker)")
	}

	// The credentials Secret is created directly instead of via the fixture directory:
	// the upgrade harness never sets cfg.Namespace(), so resources.ImportResources would
	// decode the Secret with decoder.MutateNamespace("") and the create would be rejected,
	// and VerifyResources would wait for Synced/Ready conditions a core Secret never gets.
	// No Teardown here: features run sequentially, so a teardown would delete the Secret
	// before the upgrade feature runs.
	credentialsFeature := features.New("SubaccountServiceBroker upgrade credentials").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "upgrade-test-extn-broker-credentials",
					Namespace: "default",
				},
				Type:       corev1.SecretTypeOpaque,
				StringData: map[string]string{"password": os.Getenv("SERVICE_BROKER_PASSWORD")},
			}
			if err := cfg.Client().Resources().Create(ctx, secret); err != nil && !k8serrors.IsAlreadyExists(err) {
				t.Fatalf("Failed to create broker credentials secret: %v", err)
			}
			return ctx
		}).Feature()

	upgradeTest := NewCustomUpgradeTest("subaccount-service-broker-external-name-test").
		FromVersion(brokerFromCustomTag).
		ToVersion(brokerToCustomTag).
		WithResourceDirectories(brokerCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				broker := &accountv1alpha1.SubaccountServiceBroker{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, brokerName, cfg.Namespace(), broker); err != nil {
					t.Fatalf("Failed to get SubaccountServiceBroker resource: %v", err)
				}

				annotations := broker.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist before upgrade")
				}
				if externalName == "" {
					t.Fatal("External name annotation is empty before upgrade")
				}
				// Must not be the crossplane default (the k8s metadata.name).
				if externalName == broker.GetName() {
					t.Fatalf("External name %q equals metadata.name — expected the broker GUID, not the k8s default", externalName)
				}
				// Old provider stores the bare broker GUID, so there must be no compound delimiter yet.
				if strings.Contains(externalName, "/") {
					t.Fatalf("Pre-upgrade external name %q contains %q — expected a bare broker GUID under the old provider", externalName, "/")
				}

				klog.V(4).Infof("Pre-upgrade SubaccountServiceBroker external name (bare GUID): %s", externalName)
				return context.WithValue(ctx, "preUpgradeBrokerExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				broker := &accountv1alpha1.SubaccountServiceBroker{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, brokerName, cfg.Namespace(), broker); err != nil {
					t.Fatalf("Failed to get SubaccountServiceBroker resource after upgrade: %v", err)
				}

				annotations := broker.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				// ADR(external-name): compound key "<subaccount-id>/<broker-id>" — exactly one "/",
				// both halves non-empty and valid UUIDs.
				parts := strings.Split(externalName, "/")
				if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
					t.Fatalf("Post-upgrade external name %q is not in compound form \"<subaccount-id>/<broker-id>\"", externalName)
				}
				subaccountPart, brokerPart := parts[0], parts[1]
				if !internal.IsValidUUID(subaccountPart) {
					t.Fatalf("Subaccount part %q of compound external name %q is not a valid UUID", subaccountPart, externalName)
				}
				if !internal.IsValidUUID(brokerPart) {
					t.Fatalf("Broker part %q of compound external name %q is not a valid UUID", brokerPart, externalName)
				}

				// The broker id must survive the migration unchanged.
				preUpgradeExternalName, ok := ctx.Value("preUpgradeBrokerExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}
				if brokerPart != preUpgradeExternalName {
					t.Fatalf(
						"Broker id changed during migration: pre-upgrade bare GUID %q, post-upgrade broker part %q",
						preUpgradeExternalName,
						brokerPart,
					)
				}

				// The subaccount part must equal the CR's spec.forProvider.subaccountId.
				var subaccountID string
				if broker.Spec.ForProvider.SubaccountID != nil {
					subaccountID = *broker.Spec.ForProvider.SubaccountID
				}
				if subaccountPart != subaccountID {
					t.Fatalf(
						"Subaccount part %q of compound external name does not match spec.forProvider.subaccountId %q",
						subaccountPart,
						subaccountID,
					)
				}

				klog.V(4).Infof("SubaccountServiceBroker migrated to compound external name: %s", externalName)
				return ctx
			},
		)

	testenv.Test(t, credentialsFeature, upgradeTest.Feature())
}
