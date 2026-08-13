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
	saFromCustomTag             = "v1.3.0"
	saToCustomTag               = "local"
	saCustomResourceDirectories = []string{
		upgradeCRsPath("customCRs/subaccountExternalName"),
	}
)

// Test_Subaccount_External_Name verifies that upgrading from v1.3.0 migrates the metadata.name
// external-name to the GUID of the Subaccount managed before the upgrade.
func Test_Subaccount_External_Name(t *testing.T) {
	const subaccountName = "upgrade-test-extn-sa"

	upgradeTest := NewCustomUpgradeTest("subaccount-external-name-test").
		FromVersion(saFromCustomTag).
		ToVersion(saToCustomTag).
		WithResourceDirectories(saCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify legacy external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				subaccount := &accountv1alpha1.Subaccount{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, subaccountName, cfg.Namespace(), subaccount); err != nil {
					t.Fatalf("Failed to get Subaccount resource: %v", err)
				}

				externalName, exists := subaccount.GetAnnotations()["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist")
				}

				// The old controller never set an external-name, so the runtime default stands.
				if externalName != subaccount.GetName() {
					t.Fatalf(
						"Pre-upgrade external name %q does not match the legacy sentinel (metadata.name) %q",
						externalName,
						subaccount.GetName(),
					)
				}

				if subaccount.Status.AtProvider.SubaccountGuid == nil {
					t.Fatal("status.atProvider.subaccountGuid is not set before upgrade")
				}
				guid := *subaccount.Status.AtProvider.SubaccountGuid
				if !internal.IsValidUUID(guid) {
					t.Fatalf("Pre-upgrade status.atProvider.subaccountGuid %q is not a valid UUID", guid)
				}

				klog.V(4).Infof("Pre-upgrade Subaccount external name: %s, guid: %s", externalName, guid)

				ctx = context.WithValue(ctx, "preUpgradeSaExternalName", externalName)
				return context.WithValue(ctx, "preUpgradeSaGuid", guid)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify migrated external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				preUpgradeExternalName, ok := ctx.Value("preUpgradeSaExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}
				preUpgradeGuid, ok := ctx.Value("preUpgradeSaGuid").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade subaccount guid from context")
				}

				subaccount := &accountv1alpha1.Subaccount{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, subaccountName, cfg.Namespace(), subaccount); err != nil {
					t.Fatalf("Failed to get Subaccount resource: %v", err)
				}

				externalName, exists := subaccount.GetAnnotations()["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade Subaccount external name: %s", externalName)

				if externalName == preUpgradeExternalName {
					t.Fatalf(
						"External name %q was not migrated away from the legacy sentinel",
						externalName,
					)
				}
				if !internal.IsValidUUID(externalName) {
					t.Fatalf("Post-upgrade external name %q is not a valid UUID", externalName)
				}
				if externalName != preUpgradeGuid {
					t.Fatalf(
						"Migrated external name %q does not match the pre-upgrade subaccount guid %q",
						externalName,
						preUpgradeGuid,
					)
				}

				observedGuid := ""
				if subaccount.Status.AtProvider.SubaccountGuid != nil {
					observedGuid = *subaccount.Status.AtProvider.SubaccountGuid
				}
				if observedGuid != externalName {
					t.Fatalf(
						"status.atProvider.subaccountGuid %q does not match external name %q after upgrade",
						observedGuid,
						externalName,
					)
				}

				klog.V(4).Infof(
					"External name migrated from %q to GUID %q",
					preUpgradeExternalName,
					externalName,
				)

				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
