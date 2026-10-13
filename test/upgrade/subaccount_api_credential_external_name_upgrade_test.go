//go:build upgrade

package upgrade

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	sacFromCustomTag             = "v1.3.0"
	sacToCustomTag               = "local"
	sacCustomResourceDirectories = []string{
		upgradeCRsPath("customCRs/subaccountExternalName"),
		upgradeCRsPath("customCRs/subaccountApiCredentialExternalName"),
	}
)

// Test_SubaccountApiCredential_External_Name verifies that legacy name-only
// SubaccountApiCredential external-name annotations are migrated to the ADR
// compound key "<subaccount-id>/<name>" during provider upgrades.
// This test ensures that:
// 1. The legacy managed resource is healthy before upgrade and has a complete Secret
// 2. Its external-name is normalized to the compound-key format without changing the credential-name segment
// 3. The existing Secret remains unchanged
// 4. The legacy resource becomes ownership-uncertain and is not silently promoted after upgrade
func Test_SubaccountApiCredential_External_Name(t *testing.T) {
	const sacName = "upgrade-test-extn-sac"

	upgradeTest := NewCustomUpgradeTest("subaccount-api-credential-external-name-test").
		FromVersion(sacFromCustomTag).
		ToVersion(sacToCustomTag).
		WithResourceDirectories(sacCustomResourceDirectories).
		SkipDefaultResourceVerification().
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &securityv1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{
					Name:      sacName,
					Namespace: cfg.Namespace(),
				}}
				r := cfg.Client().Resources()
				if err := wait.For(conditions.New(r).ResourceMatch(sac, func(object k8s.Object) bool {
					got, ok := object.(*securityv1alpha1.SubaccountApiCredential)
					return ok && got.GetCondition(xpv1.TypeReady).Status == corev1.ConditionTrue && got.GetCondition(xpv1.TypeSynced).Status == corev1.ConditionTrue
				}), wait.WithTimeout(8*time.Minute), wait.WithContext(ctx)); err != nil {
					t.Fatalf("wait for pre-upgrade legacy credential health: %v", err)
				}

				if err := r.Get(ctx, sacName, cfg.Namespace(), sac); err != nil {
					t.Fatalf("Failed to get SubaccountApiCredential resource: %v", err)
				}
				if sac.GetCondition(xpv1.TypeReady).Status != corev1.ConditionTrue || sac.GetCondition(xpv1.TypeSynced).Status != corev1.ConditionTrue {
					t.Fatalf("legacy credential must be healthy before upgrade: Ready=%s Synced=%s", sac.GetCondition(xpv1.TypeReady).Status, sac.GetCondition(xpv1.TypeSynced).Status)
				}

				annotations := sac.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist before upgrade")
				}
				if externalName == "" {
					t.Fatal("External name annotation is empty before upgrade")
				}

				secretRef := sac.GetWriteConnectionSecretToReference()
				if secretRef == nil || secretRef.Name == "" {
					t.Fatal("legacy credential has no connection Secret reference")
				}
				secret := &corev1.Secret{}
				if err := r.Get(ctx, secretRef.Name, secretRef.Namespace, secret); err != nil {
					t.Fatalf("get legacy connection Secret before upgrade: %v", err)
				}
				secretData := make(map[string][]byte, len(secret.Data))
				for key, value := range secret.Data {
					secretData[key] = append([]byte(nil), value...)
				}

				klog.V(4).Infof("Pre-upgrade SubaccountApiCredential external name: %s", externalName)
				ctx = context.WithValue(ctx, "preUpgradeSacSecretName", secretRef.Name)
				ctx = context.WithValue(ctx, "preUpgradeSacSecretNamespace", secretRef.Namespace)
				ctx = context.WithValue(ctx, "preUpgradeSacSecretData", secretData)
				return context.WithValue(ctx, "preUpgradeSacExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &securityv1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{
					Name:      sacName,
					Namespace: cfg.Namespace(),
				}}
				r := cfg.Client().Resources()
				if err := wait.For(conditions.New(r).ResourceMatch(sac, func(object k8s.Object) bool {
					got, ok := object.(*securityv1alpha1.SubaccountApiCredential)
					return ok && got.Status.Lifecycle != nil && got.Status.Lifecycle.Phase == securityv1alpha1.SubaccountApiCredentialPhaseUncertain
				}), wait.WithTimeout(5*time.Minute), wait.WithContext(ctx)); err != nil {
					t.Fatalf("wait for legacy ownership-uncertain classification: %v", err)
				}

				if err := r.Get(ctx, sacName, cfg.Namespace(), sac); err != nil {
					t.Fatalf("Failed to get SubaccountApiCredential resource after upgrade: %v", err)
				}

				annotations := sac.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade SubaccountApiCredential external name: %s", externalName)

				preUpgradeExternalName, ok := ctx.Value("preUpgradeSacExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				parts := strings.SplitN(externalName, "/", 2)
				if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
					t.Fatalf("Post-upgrade external-name %q is not in compound-key format", externalName)
				}
				if parts[1] != preUpgradeExternalName {
					t.Fatalf(
						"Credential-name segment changed during migration: before=%q, after compound=%q",
						preUpgradeExternalName,
						externalName,
					)
				}

				if sac.Status.Lifecycle == nil || sac.Status.Lifecycle.Phase != securityv1alpha1.SubaccountApiCredentialPhaseUncertain {
					t.Fatalf("legacy credential should remain ownership-uncertain until audited resolution; lifecycle=%#v", sac.Status.Lifecycle)
				}
				secretName, nameOK := ctx.Value("preUpgradeSacSecretName").(string)
				secretNamespace, namespaceOK := ctx.Value("preUpgradeSacSecretNamespace").(string)
				if !nameOK || !namespaceOK {
					t.Fatal("could not retrieve pre-upgrade Secret reference")
				}
				wantSecretData, ok := ctx.Value("preUpgradeSacSecretData").(map[string][]byte)
				if !ok {
					t.Fatal("could not retrieve pre-upgrade Secret data")
				}
				secret := &corev1.Secret{}
				if err := r.Get(ctx, secretName, secretNamespace, secret); err != nil {
					t.Fatalf("legacy connection Secret was not preserved after upgrade: %v", err)
				}
				if !reflect.DeepEqual(wantSecretData, secret.Data) {
					t.Errorf("legacy connection Secret data changed during identity normalization: want keys/data preserved")
				}

				klog.V(4).Info("External name migrated; legacy ownership remains uncertain and connection Secret is preserved")
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
