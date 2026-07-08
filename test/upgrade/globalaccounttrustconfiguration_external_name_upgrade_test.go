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
	gaTrustFromCustomTag             = "v1.3.0"
	gaTrustToCustomTag               = "local"
	gaTrustCustomResourceDirectories = []string{
		upgradeCRsPath("customCRs/globalaccountTrustConfigurationExternalName"),
	}
)

// Test_GlobalaccountTrustConfiguration_External_Name verifies that the external-name is preserved during upgrades.
// ADR(external-name): uses the origin key (e.g. "sap.custom") as identifier.
func Test_GlobalaccountTrustConfiguration_External_Name(t *testing.T) {
	const gaTrustName = "upgrade-test-extn-ga-trust"

	upgradeTest := NewCustomUpgradeTest("globalaccount-trust-configuration-external-name-test").
		FromVersion(gaTrustFromCustomTag).
		ToVersion(gaTrustToCustomTag).
		WithResourceDirectories(gaTrustCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				trust := &securityv1alpha1.GlobalaccountTrustConfiguration{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, gaTrustName, cfg.Namespace(), trust); err != nil {
					t.Fatalf("Failed to get GlobalaccountTrustConfiguration resource: %v", err)
				}

				annotations := trust.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist before upgrade")
				}
				if externalName == "" {
					t.Fatal("External name annotation is empty before upgrade")
				}
				// ADR(external-name): must be a real origin key, not the crossplane-runtime
				// default (metadata.name).
				if externalName == gaTrustName {
					t.Fatalf("External name %q equals metadata.name — k8s default, not an origin key", externalName)
				}

				klog.V(4).Infof("Pre-upgrade GlobalaccountTrustConfiguration external name: %s", externalName)
				return context.WithValue(ctx, "preUpgradeGaTrustExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				trust := &securityv1alpha1.GlobalaccountTrustConfiguration{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, gaTrustName, cfg.Namespace(), trust); err != nil {
					t.Fatalf("Failed to get GlobalaccountTrustConfiguration resource after upgrade: %v", err)
				}

				annotations := trust.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade GlobalaccountTrustConfiguration external name: %s", externalName)

				preUpgradeExternalName, ok := ctx.Value("preUpgradeGaTrustExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				// ADR(external-name): origin key must be preserved during upgrade — must not be migrated to another format.
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
