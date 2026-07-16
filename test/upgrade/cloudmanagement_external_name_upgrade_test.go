//go:build upgrade

package upgrade

import (
	"context"
	"strings"
	"testing"

	accountv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	"github.com/sap/crossplane-provider-btp/internal"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	// v1.11.0 is the last stable release that used DefaultSetup (bare metadata.name annotation).
	// Upgrading from it exercises the ensureCompatibility() migration to <instanceUUID>/<bindingUUID>.
	cmFromCustomTag             = "v1.11.0"
	cmToCustomTag               = "local"
	cmCustomResourceDirectories = []string{
		upgradeCRsPath("customCRs/cloudmanagementExternalName"),
	}
)

// ADR(external-name): CloudManagement migrates from bare metadata.name to compound key
// "<serviceInstanceID>/<serviceBindingID>" on upgrade.
//
// Test_CloudManagement_External_Name verifies the migration of the external-name annotation
// during a provider upgrade:
//  1. Before upgrade (old provider) the annotation is the bare metadata.name (no "/").
//  2. After upgrade the annotation is the compound key "<instanceUUID>/<bindingUUID>".
//  3. Both parts are valid UUIDs.
func Test_CloudManagement_External_Name(t *testing.T) {
	const cmName = "upgrade-test-extn-cm"

	upgradeTest := NewCustomUpgradeTest("cloudmanagement-external-name-test").
		FromVersion(cmFromCustomTag).
		ToVersion(cmToCustomTag).
		WithResourceDirectories(cmCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				cm := &accountv1beta1.CloudManagement{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, cmName, cfg.Namespace(), cm); err != nil {
					t.Fatalf("Failed to get CloudManagement resource: %v", err)
				}

				annotations := cm.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist before upgrade")
				}
				if externalName == "" {
					t.Fatal("External name annotation is empty before upgrade")
				}
				// Old provider stores the bare metadata.name, so there must be no compound delimiter yet.
				if strings.Contains(externalName, "/") {
					t.Fatalf("Pre-upgrade external name %q contains %q — expected bare metadata.name under the old provider", externalName, "/")
				}

				klog.V(4).Infof("Pre-upgrade CloudManagement external name (bare name): %s", externalName)
				return ctx
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				cm := &accountv1beta1.CloudManagement{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, cmName, cfg.Namespace(), cm); err != nil {
					t.Fatalf("Failed to get CloudManagement resource after upgrade: %v", err)
				}

				annotations := cm.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				// ADR(external-name): compound key "<instanceUUID>/<bindingUUID>" — exactly one "/",
				// both halves non-empty and valid UUIDs.
				parts := strings.Split(externalName, "/")
				if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
					t.Fatalf("Post-upgrade external name %q is not in compound form \"<instanceUUID>/<bindingUUID>\"", externalName)
				}
				if !internal.IsValidUUID(parts[0]) {
					t.Fatalf("Instance ID part %q of compound external name %q is not a valid UUID", parts[0], externalName)
				}
				if !internal.IsValidUUID(parts[1]) {
					t.Fatalf("Binding ID part %q of compound external name %q is not a valid UUID", parts[1], externalName)
				}

				klog.V(4).Infof("CloudManagement migrated to compound external name: %s", externalName)
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
