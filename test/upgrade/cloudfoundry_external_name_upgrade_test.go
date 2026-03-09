//go:build upgrade

package upgrade

import (
	"context"
	"testing"

	environmentv1alpha1 "github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	cfFromCustomTag             = "v1.1.0"
	cfToCustomTag               = "local"
	cfCustomResourceDirectories = []string{
		"./testdata/customCRs/cloudfoundryExternalName",
	}
)

// Test_CloudFoundryEnvironment_External_Name verifies that the CloudFoundryEnvironment
// external-name is correctly preserved and/or migrated during provider upgrades.
//
// The CloudFoundryEnvironment controller has backwards compatibility logic that migrates
// external-names from old formats (metadata.name in v1.0.0, orgName in > v1.1.0) to GUID format.
// This test ensures that:
// 1. The external-name exists before upgrade
// 2. After upgrade, the external-name is in GUID format
// 3. The resource remains healthy after upgrade
func Test_CloudFoundryEnvironment_External_Name(t *testing.T) {
	const cfEnvironmentName = "upgrade-test-cf-extn-env"

	upgradeTest := NewCustomUpgradeTest("cloudfoundry-environment-external-name-test").
		FromVersion(cfFromCustomTag).
		ToVersion(cfToCustomTag).
		WithResourceDirectories(cfCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				cfEnv := &environmentv1alpha1.CloudFoundryEnvironment{}
				r := cfg.Client().Resources()

				err := r.Get(ctx, cfEnvironmentName, cfg.Namespace(), cfEnv)
				if err != nil {
					t.Fatalf("Failed to get CloudFoundryEnvironment resource: %v", err)
				}

				// Get the external name annotation
				annotations := cfEnv.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]

				if !exists {
					t.Fatal("External name annotation does not exist")
				}

				klog.V(4).Infof("Pre-upgrade CloudFoundryEnvironment external name: %s", externalName)

				// Store the external name in context for post-upgrade verification
				// Note: In v1.1.0, the external-name is in orgName format, while in v1.0.0 it is in metadata.name format. Both should be migrated to GUID format after upgrade.
				return context.WithValue(ctx, "preUpgradeCfExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				cfEnv := &environmentv1alpha1.CloudFoundryEnvironment{}
				r := cfg.Client().Resources()

				err := r.Get(ctx, cfEnvironmentName, cfg.Namespace(), cfEnv)
				if err != nil {
					t.Fatalf("Failed to get CloudFoundryEnvironment resource: %v", err)
				}

				// Get the external name annotation
				annotations := cfEnv.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]

				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade CloudFoundryEnvironment external name: %s", externalName)

				// After upgrade, the external-name should be in GUID format
				// The controller migrates from orgName/metadata.name formats to GUID
				if !internal.IsValidUUID(externalName) {
					t.Fatalf("External name '%s' does not match expected UUID format after upgrade", externalName)
				}

				// Retrieve pre-upgrade external name from context
				preUpgradeExternalName, ok := ctx.Value("preUpgradeCfExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				// If the pre-upgrade external name was already a GUID, it should remain unchanged
				// If it was in orgName format, it should have been migrated to GUID format
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
					// Pre-upgrade was not GUID format (orgName or metadata.name) - should have been migrated
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
