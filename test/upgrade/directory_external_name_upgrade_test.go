//go:build upgrade

package upgrade

import (
	"context"
	"testing"

	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	dirFromCustomTag             = "v1.5.0"
	dirToCustomTag               = "local"
	dirCustomResourceDirectories = []string{
		"./testdata/customCRs/directoryExternalName",
	}
)

// Test_Directory_External_Name verifies that the Directory external-name is correctly
// preserved and/or migrated during provider upgrades.
//
// The Directory controller has backwards compatibility logic that migrates
// external-names from old formats (metadata.name prior to ADR compliance) to GUID format.
// This test ensures that:
// 1. The external-name exists before upgrade
// 2. After upgrade, the external-name is in GUID format (UUID)
// 3. The resource remains healthy after upgrade
func Test_Directory_External_Name(t *testing.T) {
	const directoryName = "upgrade-test-extn-dir"

	upgradeTest := NewCustomUpgradeTest("directory-external-name-test").
		FromVersion(dirFromCustomTag).
		ToVersion(dirToCustomTag).
		WithResourceDirectories(dirCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				directory := &accountv1alpha1.Directory{}
				r := cfg.Client().Resources()

				err := r.Get(ctx, directoryName, cfg.Namespace(), directory)
				if err != nil {
					t.Fatalf("Failed to get Directory resource: %v", err)
				}

				// Get the external name annotation
				annotations := directory.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]

				if !exists {
					t.Fatal("External name annotation does not exist")
				}

				klog.V(4).Infof("Pre-upgrade Directory external name: %s", externalName)

				// Store the external name in context for post-upgrade verification
				// Note: In older versions, the external-name may be in metadata.name format.
				// After upgrade it should be migrated to GUID format.
				return context.WithValue(ctx, "preUpgradeDirExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				directory := &accountv1alpha1.Directory{}
				r := cfg.Client().Resources()

				err := r.Get(ctx, directoryName, cfg.Namespace(), directory)
				if err != nil {
					t.Fatalf("Failed to get Directory resource: %v", err)
				}

				// Get the external name annotation
				annotations := directory.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]

				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade Directory external name: %s", externalName)

				// After upgrade, the external-name should be in GUID format (UUID)
				// The controller migrates from metadata.name format to GUID
				if !internal.IsValidUUID(externalName) {
					t.Fatalf("External name '%s' does not match expected UUID format after upgrade", externalName)
				}

				// Retrieve pre-upgrade external name from context
				preUpgradeExternalName, ok := ctx.Value("preUpgradeDirExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				// If the pre-upgrade external name was already a GUID, it should remain unchanged
				// If it was in metadata.name format, it should have been migrated to GUID format
				if internal.IsValidUUID(preUpgradeExternalName) {
					// Pre-upgrade was already GUID format - should remain the same
					if externalName != preUpgradeExternalName {
						t.Fatalf(
							"External name changed during upgrade when it shouldn't have. Before: %s, After: %s",
							preUpgradeExternalName,
							externalName,
						)
					}
					klog.V(4).Info("External name was already in GUID format and remained unchanged after upgrade")
				} else {
					// Pre-upgrade was not GUID format (metadata.name) - should have been migrated
					klog.V(4).Infof(
						"External name migrated from '%s' to GUID format '%s'",
						preUpgradeExternalName,
						externalName,
					)
				}

				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
