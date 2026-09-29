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
	// ServiceBinding external-name is a single binding GUID with no format
	// migration across releases, so this is a regression guard, not a migration
	// test: a binding created on an older release must keep its valid-GUID
	// external-name after upgrade, and the Observe UUID guard must not reject it.
	sbFromCustomTag             = "v2.1.0"
	sbToCustomTag               = "local"
	sbCustomResourceDirectories = []string{
		upgradeCRsPath("customCRs/servicebindingExternalName"),
	}
)

// Test_ServiceBinding_External_Name verifies that a ServiceBinding external-name
// survives a provider upgrade unchanged and stays a valid UUID.
//
// 1. After upgrade, the external-name is in GUID format (UUID)
// 2. If the external-name was already a GUID before upgrade, it remains unchanged
func Test_ServiceBinding_External_Name(t *testing.T) {
	const serviceBindingName = "upgrade-test-extn-sb"

	upgradeTest := NewCustomUpgradeTest("servicebinding-external-name-test").
		FromVersion(sbFromCustomTag).
		ToVersion(sbToCustomTag).
		WithResourceDirectories(sbCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sb := &accountv1alpha1.ServiceBinding{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, serviceBindingName, cfg.Namespace(), sb); err != nil {
					t.Fatalf("Failed to get ServiceBinding resource: %v", err)
				}

				externalName := sb.GetAnnotations()["crossplane.io/external-name"]
				klog.V(4).Infof("Pre-upgrade ServiceBinding external name: %s", externalName)

				return context.WithValue(ctx, "preUpgradeSbExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sb := &accountv1alpha1.ServiceBinding{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, serviceBindingName, cfg.Namespace(), sb); err != nil {
					t.Fatalf("Failed to get ServiceBinding resource: %v", err)
				}

				externalName, exists := sb.GetAnnotations()["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade ServiceBinding external name: %s", externalName)

				if !internal.IsValidUUID(externalName) {
					t.Fatalf("External name '%s' does not match expected UUID format after upgrade", externalName)
				}

				preUpgradeExternalName, ok := ctx.Value("preUpgradeSbExternalName").(string)
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
