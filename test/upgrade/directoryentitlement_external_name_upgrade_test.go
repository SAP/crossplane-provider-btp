//go:build upgrade

package upgrade

import (
	"context"
	"strings"
	"testing"

	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	directoryEntitlementFromCustomTag             = "v1.3.0"
	directoryEntitlementToCustomTag               = "local"
	directoryEntitlementCustomResourceDirectories = []string{
		"./testdata/customCRs/directoryExternalName",
		"./testdata/customCRs/directoryEntitlementExternalName",
	}
)

// Test_DirectoryEntitlement_External_Name verifies that the external-name is migrated to the new format during upgrades.
// ADR(external-name): before upgrade (v1.3.0) the external-name is the raw TF id (e.g. "cis-local");
// after upgrade it must be the compound key "<directory-id>/<service-name>/<plan-name>" (e.g. "abc-123/cis/local").
func Test_DirectoryEntitlement_External_Name(t *testing.T) {
	const directoryEntitlementName = "upgrade-test-extn-dir-ent"

	upgradeTest := NewCustomUpgradeTest("directory-entitlement-external-name-test").
		FromVersion(directoryEntitlementFromCustomTag).
		ToVersion(directoryEntitlementToCustomTag).
		WithResourceDirectories(directoryEntitlementCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				dirEnt := &accountv1alpha1.DirectoryEntitlement{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, directoryEntitlementName, cfg.Namespace(), dirEnt); err != nil {
					t.Fatalf("Failed to get DirectoryEntitlement resource: %v", err)
				}

				annotations := dirEnt.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist before upgrade")
				}

				klog.V(4).Infof("Pre-upgrade DirectoryEntitlement external name: %q", externalName)

				// Before upgrade (v1.3.0): external-name is the raw TF id "<service-name>-<plan-name>" (e.g. "cis-local")
				expectedOldFormat := *dirEnt.Spec.ForProvider.ServiceName + "-" + *dirEnt.Spec.ForProvider.PlanName
				if externalName != expectedOldFormat {
					t.Fatalf("Pre-upgrade external-name %q does not match expected old format %q", externalName, expectedOldFormat)
				}
				return context.WithValue(ctx, "preUpgradeDirEntExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				dirEnt := &accountv1alpha1.DirectoryEntitlement{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, directoryEntitlementName, cfg.Namespace(), dirEnt); err != nil {
					t.Fatalf("Failed to get DirectoryEntitlement resource after upgrade: %v", err)
				}

				annotations := dirEnt.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade DirectoryEntitlement external name: %q", externalName)

				// After upgrade: external-name must be compound key "<directory-id>/<service-name>/<plan-name>"
				if !strings.Contains(externalName, "/") {
					t.Fatalf("Post-upgrade external-name %q is not in compound-key format", externalName)
				}
				parts := strings.Split(externalName, "/")
				if len(parts) != 3 {
					t.Fatalf("Post-upgrade external-name %q must have 3 segments, got %d", externalName, len(parts))
				}
				if !internal.IsValidUUID(parts[0]) {
					t.Fatalf("Compound-key directory ID %q is not a valid UUID", parts[0])
				}
				if parts[1] != *dirEnt.Spec.ForProvider.ServiceName {
					t.Fatalf("Compound-key service name %q does not match spec %q", parts[1], *dirEnt.Spec.ForProvider.ServiceName)
				}
				if parts[2] != *dirEnt.Spec.ForProvider.PlanName {
					t.Fatalf("Compound-key plan name %q does not match spec %q", parts[2], *dirEnt.Spec.ForProvider.PlanName)
				}

				preUpgradeExternalName, ok := ctx.Value("preUpgradeDirEntExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				if preUpgradeExternalName == externalName {
					t.Fatalf("Expected external-name to be migrated; before and after both equal %q", externalName)
				}

				klog.V(4).Infof("External name migrated from %q to compound-key %q", preUpgradeExternalName, externalName)
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
