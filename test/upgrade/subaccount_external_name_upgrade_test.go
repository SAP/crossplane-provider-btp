//go:build upgrade

package upgrade

import (
	"context"
	"regexp"
	"testing"

	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	fromCustomTag             = "v1.4.0"
	toCustomTag               = "v1.5.0"
	customResourceDirectories = []string{
		"./testdata/customCRs/subaccountExternalName",
	}
	// UUID v4 format regex pattern
	uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

func Test_Subaccount_External_Name(t *testing.T) {
	const subaccountName = "upgrade-test-extn-sa"

	upgradeFeature := NewCustomUpgradeTest("subaccount-external-name-test").
		FromVersion(fromCustomTag).
		ToVersion(toCustomTag).
		WithResourceDirectories(customResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				subaccount := &accountv1alpha1.Subaccount{}
				r := cfg.Client().Resources()

				err := r.Get(ctx, subaccountName, cfg.Namespace(), subaccount)
				if err != nil {
					t.Fatalf("Failed to get Subaccount resource: %v", err)
				}

				// Get the external name annotation
				annotations := subaccount.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]

				if !exists {
					t.Fatal("External name annotation does not exist")
				}

				klog.V(4).Infof("Pre-upgrade external name: %s", externalName)

				// Verify external name matches UUID format
				if !uuidRegex.MatchString(externalName) {
					t.Fatalf("External name '%s' does not match expected UUID format", externalName)
				}

				// Store the external name in context for post-upgrade verification
				return context.WithValue(ctx, "preUpgradeExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				subaccount := &accountv1alpha1.Subaccount{}
				r := cfg.Client().Resources()

				err := r.Get(ctx, subaccountName, cfg.Namespace(), subaccount)
				if err != nil {
					t.Fatalf("Failed to get Subaccount resource: %v", err)
				}

				// Get the external name annotation
				annotations := subaccount.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]

				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade external name: %s", externalName)

				// Verify external name matches UUID format
				if !uuidRegex.MatchString(externalName) {
					t.Fatalf("External name '%s' does not match expected UUID format after upgrade", externalName)
				}

				// Verify external name hasn't changed during upgrade
				preUpgradeExternalName, ok := ctx.Value("preUpgradeExternalName").(string)
				if !ok {
					t.Fatal("Could not retrieve pre-upgrade external name from context")
				}

				if externalName != preUpgradeExternalName {
					t.Fatalf(
						"External name changed during upgrade. Before: %s, After: %s",
						preUpgradeExternalName,
						externalName,
					)
				}

				return ctx
			},
		).
		Build()

	testenv.Test(t, upgradeFeature)
}
