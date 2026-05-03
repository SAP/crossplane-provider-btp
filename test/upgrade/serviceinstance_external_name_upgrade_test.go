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
	// v1.9.0 used Terraform import() for ServiceInstance adoption, requiring a different external-name format.
	// This branch switches to Terraform refresh() with management policy "*", which requires the external-name
	// to be the ServiceInstance GUID (UUID format) → we test from the version before to the current one.
	siFromCustomTag             = "v1.9.0"
	siToCustomTag               = "local"
	siCustomResourceDirectories = []string{
		"./testdata/customCRs/serviceinstanceExternalName",
	}
)

// Test_ServiceInstance_External_Name verifies that the ServiceInstance external-name is correctly
// set during provider upgrades.
//
// Before this change (v1.9.0 and earlier), ServiceInstance used Terraform import() to adopt existing
// resources. With this change, the controller switches to Terraform refresh() (management policy "*")
// and requires the external-name to be set to the ServiceInstance GUID (UUID format).
// This test ensures that after upgrading from the old import()-based behavior to the new refresh()-based
// behavior, the external-name is correctly set to a valid UUID and the resource remains healthy.
// This test ensures that:
// 1. After upgrade, the external-name is in GUID format (UUID)
// 2. If the external-name was already a GUID before upgrade, it remains unchanged
func Test_ServiceInstance_External_Name(t *testing.T) {
	const serviceInstanceName = "upgrade-test-extn-si"

	upgradeTest := NewCustomUpgradeTest("serviceinstance-external-name-test").
		FromVersion(siFromCustomTag).
		ToVersion(siToCustomTag).
		WithResourceDirectories(siCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				si := &accountv1alpha1.ServiceInstance{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, serviceInstanceName, cfg.Namespace(), si); err != nil {
					t.Fatalf("Failed to get ServiceInstance resource: %v", err)
				}

				// In v1.9.0 and earlier, the external-name annotation may not be set
				annotations := si.GetAnnotations()
				externalName := annotations["crossplane.io/external-name"]

				klog.V(4).Infof("Pre-upgrade ServiceInstance external name: %s", externalName)

				return context.WithValue(ctx, "preUpgradeSiExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				si := &accountv1alpha1.ServiceInstance{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, serviceInstanceName, cfg.Namespace(), si); err != nil {
					t.Fatalf("Failed to get ServiceInstance resource: %v", err)
				}

				annotations := si.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]

				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade ServiceInstance external name: %s", externalName)

				if !internal.IsValidUUID(externalName) {
					t.Fatalf("External name '%s' does not match expected UUID format after upgrade", externalName)
				}

				preUpgradeExternalName, ok := ctx.Value("preUpgradeSiExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				if internal.IsValidUUID(preUpgradeExternalName) {
					if externalName != preUpgradeExternalName {
						t.Fatalf(
							"External name changed during upgrade when it shouldn't have. Before: %s, After: %s",
							preUpgradeExternalName,
							externalName,
						)
					}
					klog.V(4).Info("External name was already in GUID format and remained unchanged after upgrade")
				} else {
					klog.V(4).Infof("External name set to GUID format '%s' after upgrade", externalName)
				}

				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
