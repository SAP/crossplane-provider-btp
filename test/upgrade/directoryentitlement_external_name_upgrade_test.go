//go:build upgrade

package upgrade

import (
	"context"
	"regexp"
	"testing"

	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
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

// Test_DirectoryEntitlement_External_Name verifies that the external-name is preserved during upgrades.
// ADR(external-name): uses compound key "<directory-id>/<service-name>/<plan-name>" (e.g. "abc-123/cis/local") as identifier.
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
				if externalName == "" {
					t.Fatal("External name annotation is empty before upgrade")
				}
				if matched, _ := regexp.MatchString(`^[^/]+/[^/]+/[^/]+$`, externalName); !matched {
					t.Fatalf("External name %q does not match expected format <directory-id>/<service-name>/<plan-name> before upgrade", externalName)
				}

				klog.V(4).Infof("Pre-upgrade DirectoryEntitlement external name: %s", externalName)
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

				klog.V(4).Infof("Post-upgrade DirectoryEntitlement external name: %s", externalName)

				preUpgradeExternalName, ok := ctx.Value("preUpgradeDirEntExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				// ADR(external-name): compound key "<directory-id>/<service-name>/<plan-name>" must be preserved during upgrade — must not be migrated to plan-id format.
				if externalName != preUpgradeExternalName {
					t.Fatalf(
						"External name changed during upgrade: before=%q, after=%q (expected no change)",
						preUpgradeExternalName,
						externalName,
					)
				}

				klog.V(4).Info("External name correctly preserved after upgrade")
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
