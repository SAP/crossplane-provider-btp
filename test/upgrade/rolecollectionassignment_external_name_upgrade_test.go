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
	rcaFromCustomTag             = "v1.9.0"
	rcaToCustomTag               = "local"
	rcaCustomResourceDirectories = []string{
		"./testdata/customCRs/subaccountExternalName",
		"./testdata/customCRs/subaccountApiCredentialExternalName",
		"./testdata/customCRs/rolecollectionAssignmentExternalName",
	}
)

// Test_RoleCollectionAssignment_External_Name verifies that the
// RoleCollectionAssignment external-name is correctly migrated during provider
// upgrades.
//
// ADR(external-name): RoleCollectionAssignment uses the compound key
// "<origin>/<userOrGroup>/<roleCollection>" as identifier. Pre-ADR controllers
// let crossplane-runtime default the annotation to metadata.name (or left it
// empty if suppressed). This test ensures the upgrade migrates legacy
// annotations to the compound-key format.
func Test_RoleCollectionAssignment_External_Name(t *testing.T) {
	const rcaName = "upgrade-test-extn-rca"

	upgradeTest := NewCustomUpgradeTest("rolecollection-assignment-external-name-test").
		FromVersion(rcaFromCustomTag).
		ToVersion(rcaToCustomTag).
		WithResourceDirectories(rcaCustomResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				rca := &securityv1alpha1.RoleCollectionAssignment{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, rcaName, cfg.Namespace(), rca); err != nil {
					t.Fatalf("Failed to get RoleCollectionAssignment resource: %v", err)
				}

				annotations := rca.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist before upgrade")
				}

				klog.V(4).Infof("Pre-upgrade RoleCollectionAssignment external name: %q", externalName)
				return context.WithValue(ctx, "preUpgradeRcaExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				rca := &securityv1alpha1.RoleCollectionAssignment{}
				r := cfg.Client().Resources()

				if err := r.Get(ctx, rcaName, cfg.Namespace(), rca); err != nil {
					t.Fatalf("Failed to get RoleCollectionAssignment resource after upgrade: %v", err)
				}

				annotations := rca.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]
				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade RoleCollectionAssignment external name: %q", externalName)

				// After upgrade, the external-name must be the compound key
				// "<origin>/<userOrGroup>/<roleCollection>".
				if !strings.Contains(externalName, "/") {
					t.Fatalf("Post-upgrade external-name %q is not in compound-key format", externalName)
				}
				parts := strings.Split(externalName, "/")
				if len(parts) != 3 {
					t.Fatalf("Post-upgrade external-name %q must have 3 segments, got %d", externalName, len(parts))
				}
				if parts[0] != rca.Spec.ForProvider.Origin {
					t.Fatalf("Compound-key origin %q does not match spec %q", parts[0], rca.Spec.ForProvider.Origin)
				}
				if parts[2] != rca.Spec.ForProvider.RoleCollectionName {
					t.Fatalf("Compound-key roleCollection %q does not match spec %q", parts[2], rca.Spec.ForProvider.RoleCollectionName)
				}

				preUpgradeExternalName, ok := ctx.Value("preUpgradeRcaExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				if preUpgradeExternalName == externalName {
					t.Fatalf("Expected external-name to be migrated; before and after both equal %q", externalName)
				}

				klog.V(4).Infof("External name migrated from %q to compound-key %q", preUpgradeExternalName, externalName)
				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}
