//go:build upgrade

package upgrade

import (
	"context"
	"strings"
	"testing"

	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	sacFromCustomTag             = "v1.3.0"
	sacToCustomTag               = "local"
	sacCustomResourceDirectories = []string{
		upgradeCRsPath("customCRs/subaccountExternalName"),
		upgradeCRsPath("customCRs/subaccountApiCredentialExternalName"),
	}
)

// Test_SubaccountApiCredential_External_Name verifies that legacy name-only
// SubaccountApiCredential external-name annotations are migrated to the ADR
// compound key "<subaccount-id>/<name>" during provider upgrades.
// This test ensures that:
// 1. The pre-upgrade external-name exists in the legacy name-only format
// 2. After upgrade, the external-name is migrated to the compound-key format
// 3. The credential-name segment is preserved, so the existing external resource remains targeted
// 4. The resource remains healthy after upgrade
func Test_SubaccountApiCredential_External_Name(t *testing.T) {
	const sacName = "upgrade-test-extn-sac"

	upgradeTest := NewCustomUpgradeTest("subaccount-api-credential-external-name-test").
		FromVersion(sacFromCustomTag).
		ToVersion(sacToCustomTag).
		WithResourceDirectories(sacCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &securityv1alpha1.SubaccountApiCredential{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, sacName, cfg.Namespace(), sac); err != nil {
					t.Fatalf("Failed to get SubaccountApiCredential resource: %v", err)
				}

				annotations := sac.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist before upgrade")
				}
				if externalName == "" {
					t.Fatal("External name annotation is empty before upgrade")
				}

				klog.V(4).Infof("Pre-upgrade SubaccountApiCredential external name: %s", externalName)
				return context.WithValue(ctx, "preUpgradeSacExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &securityv1alpha1.SubaccountApiCredential{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, sacName, cfg.Namespace(), sac); err != nil {
					t.Fatalf("Failed to get SubaccountApiCredential resource after upgrade: %v", err)
				}

				annotations := sac.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade SubaccountApiCredential external name: %s", externalName)

				preUpgradeExternalName, ok := ctx.Value("preUpgradeSacExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				parts := strings.SplitN(externalName, "/", 2)
				if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
					t.Fatalf("Post-upgrade external-name %q is not in compound-key format", externalName)
				}
				if parts[1] != preUpgradeExternalName {
					t.Fatalf(
						"Credential-name segment changed during migration: before=%q, after compound=%q",
						preUpgradeExternalName,
						externalName,
					)
				}

				klog.V(4).Info("External name correctly migrated after upgrade")
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
