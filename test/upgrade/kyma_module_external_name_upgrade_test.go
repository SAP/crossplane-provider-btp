//go:build upgrade

package upgrade

import (
	"context"
	"testing"

	environmentv1alpha1 "github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	// v1.8.0 used forProvider.name as the identifier in Observe/Delete but external-name was set on Create.
	// current version uses external-name annotation for both Observe and Delete (ADR compliance).
	kymaModuleFromCustomTag             = "v1.8.0"
	kymaModuleToCustomTag               = "local"
	kymaModuleCustomResourceDirectories = []string{
		"./testdata/customCRs/kymaModuleExternalName",
	}
)

// Test_KymaModule_External_Name verifies that the KymaModule external-name
// is correctly preserved during provider upgrades.
//
// KymaModule external-name is the module name (e.g. "cloud-manager").
// This was set on Create in all versions. This test verifies that:
// 1. The external-name is set to the module name before upgrade
// 2. After upgrade (ADR compliance), the external-name is still the module name
// 3. The resource remains healthy after upgrade
func Test_KymaModule_External_Name(t *testing.T) {
	const kymaModuleName = "upgrade-test-kyma-module"
	const expectedModuleName = "cloud-manager"

	upgradeTest := NewCustomUpgradeTest("kyma-module-external-name-test").
		FromVersion(kymaModuleFromCustomTag).
		ToVersion(kymaModuleToCustomTag).
		WithResourceDirectories(kymaModuleCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				kymaModule := &environmentv1alpha1.KymaModule{}
				r := cfg.Client().Resources()

				err := r.Get(ctx, kymaModuleName, cfg.Namespace(), kymaModule)
				if err != nil {
					t.Fatalf("Failed to get KymaModule resource: %v", err)
				}

				annotations := kymaModule.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist before upgrade")
				}

				klog.V(4).Infof("Pre-upgrade KymaModule external name: %s", externalName)

				if externalName != expectedModuleName {
					t.Fatalf("Expected external name '%s' before upgrade, got '%s'", expectedModuleName, externalName)
				}

				return context.WithValue(ctx, "preUpgradeKymaModuleExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				kymaModule := &environmentv1alpha1.KymaModule{}
				r := cfg.Client().Resources()

				err := r.Get(ctx, kymaModuleName, cfg.Namespace(), kymaModule)
				if err != nil {
					t.Fatalf("Failed to get KymaModule resource after upgrade: %v", err)
				}

				annotations := kymaModule.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade KymaModule external name: %s", externalName)

				// After upgrade, the external-name must still be the module name
				if externalName != expectedModuleName {
					t.Fatalf("Expected external name '%s' after upgrade, got '%s'", expectedModuleName, externalName)
				}

				// Verify it matches the pre-upgrade value (no change expected)
				preUpgradeExternalName, ok := ctx.Value("preUpgradeKymaModuleExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				if externalName != preUpgradeExternalName {
					t.Fatalf(
						"External name changed during upgrade when it shouldn't have. Before: %s, After: %s",
						preUpgradeExternalName,
						externalName,
					)
				}

				klog.V(4).Infof("KymaModule external name correctly preserved as '%s' after upgrade", externalName)
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
