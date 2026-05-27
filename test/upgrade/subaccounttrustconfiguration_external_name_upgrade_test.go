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
	trustFromCustomTag             = "v1.3.0"
	trustToCustomTag               = "local"
	trustCustomResourceDirectories = []string{
		"./testdata/customCRs/subaccountExternalName",
		"./testdata/customCRs/subaccountTrustConfigurationExternalName",
	}
)

// Test_SubaccountTrustConfiguration_External_Name verifies that the external-name is preserved during upgrades.
// ADR(external-name): uses compound key "<subaccount-id>,<origin>" (e.g. "abc-123,sap.custom") as identifier.
func Test_SubaccountTrustConfiguration_External_Name(t *testing.T) {
	const trustName = "upgrade-test-extn-trust"

	upgradeTest := NewCustomUpgradeTest("subaccount-trust-configuration-external-name-test").
		FromVersion(trustFromCustomTag).
		ToVersion(trustToCustomTag).
		WithResourceDirectories(trustCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				trust := &securityv1alpha1.SubaccountTrustConfiguration{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, trustName, cfg.Namespace(), trust); err != nil {
					t.Fatalf("Failed to get SubaccountTrustConfiguration resource: %v", err)
				}

				annotations := trust.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist before upgrade")
				}
				if externalName == "" {
					t.Fatal("External name annotation is empty before upgrade")
				}

				klog.V(4).Infof("Pre-upgrade SubaccountTrustConfiguration external name: %s", externalName)
				return context.WithValue(ctx, "preUpgradeTrustExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				trust := &securityv1alpha1.SubaccountTrustConfiguration{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, trustName, cfg.Namespace(), trust); err != nil {
					t.Fatalf("Failed to get SubaccountTrustConfiguration resource after upgrade: %v", err)
				}

				annotations := trust.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade SubaccountTrustConfiguration external name: %s", externalName)

				preUpgradeExternalName, ok := ctx.Value("preUpgradeTrustExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				// ADR(external-name): compound key "<subaccount-id>,<origin>" must be preserved during upgrade — must not be migrated to UUID format.
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
