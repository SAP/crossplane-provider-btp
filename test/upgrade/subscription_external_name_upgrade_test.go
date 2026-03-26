//go:build upgrade

package upgrade

import (
	"context"
	"strings"
	"testing"

	xpmeta "github.com/crossplane/crossplane-runtime/pkg/meta"
	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

func Test_Subscription_External_Name_Migration(t *testing.T) {
	const subscriptionName = "upgrade-test-extn-subscription"

	var (
		fromCustomTag             = "v1.0.0"
		toCustomTag               = "local"
		customResourceDirectories = []string{
			"./testdata/customCRs/subscriptionExternalName",
		}
	)

	upgradeTest := NewCustomUpgradeTest("subscription-external-name-migration-test").
		FromVersion(fromCustomTag).
		ToVersion(toCustomTag).
		WithResourceDirectories(customResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external-name before upgrade (old format)",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				subscription := &accountv1alpha1.Subscription{}
				r := cfg.Client().Resources()

				err := r.Get(ctx, subscriptionName, cfg.Namespace(), subscription)
				if err != nil {
					t.Fatalf("Failed to get Subscription resource: %v", err)
				}

				// Get the external name annotation
				externalName := xpmeta.GetExternalName(subscription)
				klog.V(4).Infof("Pre-upgrade external-name: %s", externalName)

				// Before upgrade, external-name should equal metadata.name (old default behavior)
				if externalName != subscriptionName {
					t.Logf("Warning: External-name '%s' doesn't match metadata.name '%s'. This might be expected if testing from a version that already has the fix.", externalName, subscriptionName)
				}

				// Store values for post-upgrade verification
				ctx = context.WithValue(ctx, "preUpgradeExternalName", externalName)
				ctx = context.WithValue(ctx, "appName", subscription.Spec.ForProvider.AppName)
				ctx = context.WithValue(ctx, "planName", subscription.Spec.ForProvider.PlanName)

				return ctx
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external-name after upgrade (migrated to new format)",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				subscription := &accountv1alpha1.Subscription{}
				r := cfg.Client().Resources()

				err := r.Get(ctx, subscriptionName, cfg.Namespace(), subscription)
				if err != nil {
					t.Fatalf("Failed to get Subscription resource: %v", err)
				}

				// Get the external name annotation after upgrade
				externalName := xpmeta.GetExternalName(subscription)
				klog.V(4).Infof("Post-upgrade external-name: %s", externalName)

				// Retrieve pre-upgrade values from context
				appName, _ := ctx.Value("appName").(string)
				planName, _ := ctx.Value("planName").(string)

				// After upgrade, external-name should be in appName/planName format
				expectedExternalName := appName + "/" + planName

				if !isValidSubscriptionExternalName(externalName) {
					t.Fatalf("External-name '%s' does not match expected format <appName>/<planName> after upgrade", externalName)
				}

				// Verify it was migrated to the correct format
				if externalName != expectedExternalName {
					t.Fatalf(
						"External-name was not migrated correctly. Expected: %s, Got: %s",
						expectedExternalName,
						externalName,
					)
				}

				// Verify the subscription is still healthy after migration
				if subscription.Status.AtProvider.State == nil {
					t.Fatal("Subscription state is nil after upgrade")
				}

				klog.V(4).Infof("Successfully verified external-name migration from old format to new format: %s", externalName)

				return ctx
			},
		)

	testenv.Test(t, upgradeTest.Feature())
}

// isValidSubscriptionExternalName validates that the external name is in the format appName/planName
func isValidSubscriptionExternalName(externalName string) bool {
	parts := strings.Split(externalName, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}
