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
		upgradeCRsPath("customCRs/directoryExternalName"),
	}
)

// Test_Directory_External_Name verifies that upgrading from v1.5.0 preserves the directory GUID
// external-name and keeps it aligned with status.atProvider.guid.
//
// Directory has no legacy external-name migration path. The upgraded controller rejects a
// non-empty external-name unless it is a UUID.
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

				if err := r.Get(ctx, directoryName, cfg.Namespace(), directory); err != nil {
					t.Fatalf("Failed to get Directory resource: %v", err)
				}

				externalName, exists := directory.GetAnnotations()["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist")
				}

				// The old controller set the external-name from the create response.
				if externalName == directory.GetName() {
					t.Fatalf(
						"Pre-upgrade external name %q still equals metadata.name; expected the directory GUID",
						externalName,
					)
				}
				if !internal.IsValidUUID(externalName) {
					t.Fatalf("Pre-upgrade external name %q is not a valid UUID", externalName)
				}

				observedGuid := ""
				if directory.Status.AtProvider.Guid != nil {
					observedGuid = *directory.Status.AtProvider.Guid
				}
				if observedGuid != externalName {
					t.Fatalf(
						"Pre-upgrade status.atProvider.guid %q does not match external name %q",
						observedGuid,
						externalName,
					)
				}

				klog.V(4).Infof("Pre-upgrade Directory external name: %s", externalName)

				return context.WithValue(ctx, "preUpgradeDirExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name is preserved after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				preUpgradeExternalName, ok := ctx.Value("preUpgradeDirExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				directory := &accountv1alpha1.Directory{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, directoryName, cfg.Namespace(), directory); err != nil {
					t.Fatalf("Failed to get Directory resource: %v", err)
				}

				externalName, exists := directory.GetAnnotations()["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade Directory external name: %s", externalName)

				if externalName != preUpgradeExternalName {
					t.Fatalf(
						"External name changed during upgrade. Before: %q, after: %q",
						preUpgradeExternalName,
						externalName,
					)
				}

				observedGuid := ""
				if directory.Status.AtProvider.Guid != nil {
					observedGuid = *directory.Status.AtProvider.Guid
				}
				if observedGuid != externalName {
					t.Fatalf(
						"Post-upgrade status.atProvider.guid %q does not match external name %q",
						observedGuid,
						externalName,
					)
				}

				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
