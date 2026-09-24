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
	trustFromCustomTag             = "v1.3.0"
	trustToCustomTag               = "local"
	trustCustomResourceDirectories = []string{
		upgradeCRsPath("customCRs/subaccountExternalName"),
		upgradeCRsPath("customCRs/subaccountTrustConfigurationExternalName"),
	}
)

// Test_SubaccountTrustConfiguration_External_Name verifies that the
// SubaccountTrustConfiguration external-name is correctly migrated during
// provider upgrades.
//
// ADR(external-name): SubaccountTrustConfiguration uses the compound key
// "<subaccount-id>/<origin>" as identifier (see
// config/subaccount_trust_configuration/config.go)
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

				// After upgrade, the external-name must be the compound key
				// "<subaccount-id>/<origin>".
				parts := strings.SplitN(externalName, "/", 2)
				if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
					t.Fatalf("Post-upgrade external-name %q is not in compound-key format \"<subaccount-id>/<origin>\"", externalName)
				}
				subaccountSegment, originSegment := parts[0], parts[1]

				// The origin segment of the compound key must match
				// status.atProvider.origin, which the controller populates from
				// tfstate after the first Observe.
				if trust.Status.AtProvider.Origin == nil || *trust.Status.AtProvider.Origin == "" {
					t.Fatal("status.atProvider.origin is not populated after upgrade")
				}
				if originSegment != *trust.Status.AtProvider.Origin {
					t.Fatalf(
						"Compound-key origin segment %q does not match status.atProvider.origin %q",
						originSegment,
						*trust.Status.AtProvider.Origin,
					)
				}

				// The subaccount segment must match status.atProvider.subaccountId,
				// which the controller populates from tfstate after the first Observe.
				if trust.Status.AtProvider.SubaccountID == nil || *trust.Status.AtProvider.SubaccountID == "" {
					t.Fatal("status.atProvider.subaccountId is not populated after upgrade")
				}
				if subaccountSegment != *trust.Status.AtProvider.SubaccountID {
					t.Fatalf(
						"Compound-key subaccount segment %q does not match status.atProvider.subaccountId %q",
						subaccountSegment,
						*trust.Status.AtProvider.SubaccountID,
					)
				}

				klog.V(4).Infof("External name migrated from %q to compound-key %q", preUpgradeExternalName, externalName)
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
