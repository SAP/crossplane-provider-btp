//go:build upgrade

package upgrade

import (
	"context"
	"testing"

	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	sacFromCustomTag             = "v1.3.0"
	sacToCustomTag               = "local"
	sacCustomResourceDirectories = []string{
		"./testdata/customCRs/subaccountExternalName",
		"./testdata/customCRs/subaccountApiCredentialExternalName",
	}
)

// Test_SubaccountApiCredential_External_Name verifies that the SubaccountApiCredential
// external-name is correctly preserved during provider upgrades.
//
// Unlike most resources, SubaccountApiCredential is an ADR exception: the SAP BTP Terraform
// provider uses the credential name (not a GUID) as its resource identifier. Therefore the
// external-name is the credential name (e.g. "my-api-credential"), and no migration to UUID
// format is expected. This test ensures that:
// 1. The external-name exists before upgrade and equals the credential name
// 2. After upgrade, the external-name is unchanged
// 3. The resource remains healthy after upgrade
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

				// The external-name must be unchanged: SubaccountApiCredential uses the
				// credential name as its Terraform resource identifier (ADR exception).
				// No migration to UUID format should occur.
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
