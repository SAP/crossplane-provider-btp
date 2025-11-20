//go:build upgrade

package upgrade

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/upgrade"
	"github.com/sap/crossplane-provider-btp/test"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestUpgradeProvider(t *testing.T) {
	upgradeTest := upgrade.UpgradeTest{
		ClusterName:         kindClusterName,
		ProviderName:        providerName,
		FromProviderPackage: fromPackage,
		ToProviderPackage:   toPackage,
		ResourceDirectories: resourceDirectories,
	}

	upgradeFeature := features.New(fmt.Sprintf("upgrade provider btp from %s to %s", fromTag, toTag)).
		WithSetup(
			"install provider",
			upgrade.ApplyProvider(upgradeTest.ClusterName, upgradeTest.FromProviderInstallOptions()),
		).
		WithSetup(
			"import resources",
			upgrade.ImportResources(upgradeTest.ResourceDirectories),
		).
		Assess(
			"verify resources before upgrade",
			upgrade.VerifyResources(upgradeTest.ResourceDirectories, time.Minute*30),
		).
		Assess("upgrade provider", upgrade.UpgradeProvider(upgrade.UpgradeProviderOptions{
			ClusterName:         upgradeTest.ClusterName,
			ProviderOptions:     upgradeTest.ToProviderInstallOptions(),
			ResourceDirectories: upgradeTest.ResourceDirectories,
			WaitForPause:        time.Minute * 1,
		})).
		Assess(
			"verify resources after upgrade",
			upgrade.VerifyResources(upgradeTest.ResourceDirectories, time.Minute*30),
		).
		WithTeardown(
			"delete resources",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				err := test.DeleteResourcesFromDirsGracefully(ctx, cfg, resourceDirectories, wait.WithTimeout(time.Minute*30))
				if err != nil {
					t.Logf("failed to clean up resources: %v", err)
				}

				return ctx
			},
		).
		WithTeardown(
			"delete provider",
			upgrade.DeleteProvider(upgradeTest.ProviderName),
		)
	testenv.Test(t, upgradeFeature.Feature())
}
