//go:build upgrade

package upgrade

import (
	"context"
	"strings"
	"testing"

	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

func Test_Subscription_External_Name(t *testing.T) {
	const subscriptionName = "upgrade-test-extn-subscription"

	var (
		fromCustomTag             = "v1.5.0" // Version before external name implementation
		toCustomTag               = "main"   // Current version with external name support
		customResourceDirectories = []string{
			"./testdata/customCRs/subscriptionExternalName",
		}
	)

	upgradeTest := NewCustomUpgradeTest("subscription-external-name-test").
		FromVersion(fromCustomTag).
		ToVersion(toCustomTag).
		WithResourceDirectories(customResourceDirectories).
		WithCustomPreUpgradeAssessment(
			"verify external name before upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				subscription := &accountv1alpha1.Subscription{}
				r := cfg.Client().Resources()

				err := r.Get(ctx, subscriptionName, cfg.Namespace(), subscription)
				if err != nil {
					t.Fatalf("Failed to get Subscription resource: %v", err)
				}

				// Get the external name annotation
				annotations := subscription.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]

				if !exists {
					t.Fatal("External name annotation does not exist")
				}

				klog.V(4).Infof("Pre-upgrade external name: %s", externalName)

				// Verify external name matches format <appName>/<planName>
				if !isValidSubscriptionExternalName(externalName) {
					t.Fatalf("External name '%s' does not match expected format <appName>/<planName>", externalName)
				}

				// Store the external name in context for post-upgrade verification
				return context.WithValue(ctx, "preUpgradeExternalName", externalName)
			},
		).
		WithCustomPostUpgradeAssessment(
			"verify external name after upgrade",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				subscription := &accountv1alpha1.Subscription{}
				r := cfg.Client().Resources()

				err := r.Get(ctx, subscriptionName, cfg.Namespace(), subscription)
				if err != nil {
					t.Fatalf("Failed to get Subscription resource: %v", err)
				}

				// Get the external name annotation
				annotations := subscription.GetAnnotations()
				externalName, exists := annotations["crossplane.io/external-name"]

				if !exists {
					t.Fatal("External name annotation does not exist after upgrade")
				}

				klog.V(4).Infof("Post-upgrade external name: %s", externalName)

				// Verify external name matches format <appName>/<planName>
				if !isValidSubscriptionExternalName(externalName) {
					t.Fatalf("External name '%s' does not match expected format <appName>/<planName> after upgrade", externalName)
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
		)

	testenv.Test(t, upgradeTest.Feature())
}

// isValidSubscriptionExternalName validates that the external name is in the format appName/planName
func isValidSubscriptionExternalName(externalName string) bool {
	parts := strings.Split(externalName, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}
