//go:build upgrade

package upgrade

import (
	"context"
	"strings"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	entitlementFromCustomTag             = "v1.10.0"
	entitlementToCustomTag               = "local"
	entitlementCustomResourceDirectories = []string{
		upgradeCRsPath("customCRs/entitlementExternalName"),
	}
)

// Test_Entitlement_External_Name verifies that the external-name is migrated to the
// ADR-compliant compound key during upgrades.
// ADR(external-name): before upgrade (v1.10.0), external-name is the legacy sentinel
// (metadata.name, e.g. "upgrade-test-extn-ent"); after upgrade it must be the compound key
// "<subaccount-guid>/<service-name>/<service-plan-name>" (e.g. "abc-123/postgresql-db/development").
// The fixture omits servicePlanUniqueIdentifier, so the migrated key has exactly three segments.
func Test_Entitlement_External_Name(t *testing.T) {
	const entitlementName = "upgrade-test-extn-ent"

	upgradeTest := NewCustomUpgradeTest("entitlement-external-name-test").
		FromVersion(entitlementFromCustomTag).
		ToVersion(entitlementToCustomTag).
		WithResourceDirectories(entitlementCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				ent := &accountv1alpha1.Entitlement{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, entitlementName, cfg.Namespace(), ent); err != nil {
					t.Fatalf("Failed to get Entitlement resource: %v", err)
				}

				annotations := ent.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist before upgrade")
				}

				klog.V(4).Infof("Pre-upgrade Entitlement external name: %q", externalName)

				// Before upgrade (v1.10.0): crossplane-runtime's default initializer has
				// stamped external-name with the legacy sentinel, metadata.name.
				if externalName != ent.Name {
					t.Fatalf("Pre-upgrade external-name %q does not match expected legacy sentinel (metadata.name) %q", externalName, ent.Name)
				}
				return context.WithValue(ctx, "preUpgradeEntExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				ent := &accountv1alpha1.Entitlement{}
				ent.SetName(entitlementName)
				ent.SetNamespace(cfg.Namespace())
				r := cfg.Client().Resources()

				// Fail fast on a missing CR, wrong name, or RBAC error instead of
				// letting ResourceMatch (which swallows Get errors) burn the whole
				// wait timeout and mask the real cause.
				if err := r.Get(ctx, entitlementName, cfg.Namespace(), ent); err != nil {
					t.Fatalf("Failed to get Entitlement resource after upgrade: %v", err)
				}

				// Gate on both Synced and Ready: PauseResources only flips Synced during
				// the provider swap, so Ready alone could still reflect the pre-upgrade
				// state. ResourceMatch refetches ent on every poll, so ent reflects the
				// matching revision once the wait succeeds.
				if err := wait.For(
					conditions.New(r).ResourceMatch(ent, func(object k8s.Object) bool {
						e := object.(*accountv1alpha1.Entitlement)
						synced := e.GetCondition(xpv1.TypeSynced)
						ready := e.GetCondition(xpv1.TypeReady)
						return synced.Status == corev1.ConditionTrue && ready.Status == corev1.ConditionTrue
					}),
					wait.WithTimeout(globalVerifyTimeout),
				); err != nil {
					t.Fatalf("Entitlement did not become Synced and Ready after upgrade: %v", err)
				}

				annotations := ent.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade Entitlement external name: %q", externalName)

				// After upgrade: external-name must be the compound key
				// "<subaccount-guid>/<service-name>/<service-plan-name>". The fixture sets
				// no servicePlanUniqueIdentifier, so exactly three segments are expected.
				parts := strings.Split(externalName, "/")
				if len(parts) != 3 {
					t.Fatalf("Post-upgrade external-name %q must have 3 segments, got %d", externalName, len(parts))
				}
				if parts[0] != ent.Spec.ForProvider.SubaccountGuid {
					t.Fatalf("Compound-key subaccount guid %q does not match spec %q", parts[0], ent.Spec.ForProvider.SubaccountGuid)
				}
				if parts[1] != ent.Spec.ForProvider.ServiceName {
					t.Fatalf("Compound-key service name %q does not match spec %q", parts[1], ent.Spec.ForProvider.ServiceName)
				}
				if parts[2] != ent.Spec.ForProvider.ServicePlanName {
					t.Fatalf("Compound-key service plan name %q does not match spec %q", parts[2], ent.Spec.ForProvider.ServicePlanName)
				}

				preUpgradeExternalName, ok := ctx.Value("preUpgradeEntExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				if preUpgradeExternalName == externalName {
					t.Fatalf("Expected external-name to be migrated; before and after both equal %q", externalName)
				}

				klog.V(4).Infof("External name migrated from %q to compound-key %q", preUpgradeExternalName, externalName)

				// Delete the Entitlement before teardown so the Subaccount is not deleted
				// while the entitlement still references it.
				//
				// The 20-minute timeout accounts for BTP's asynchronous assignment removal:
				// SetServicePlans only requests removal, and this controller's poll cycle
				// notices completion on a later reconcile once BTP's state reflects it.
				klog.V(4).Info("Deleting Entitlement before teardown")
				if err := r.Delete(ctx, ent); err != nil {
					t.Fatalf("Failed to delete Entitlement: %v", err)
				}
				if err := wait.For(
					conditions.New(r).ResourceDeleted(ent),
					wait.WithTimeout(20*time.Minute),
				); err != nil {
					t.Fatalf("Entitlement was not deleted within timeout: %v", err)
				}
				klog.V(4).Info("Entitlement deleted before teardown")

				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
