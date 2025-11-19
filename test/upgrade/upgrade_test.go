//go:build upgrade

package upgrade

import (
	"fmt"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/upgrade"
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
			upgrade.VerifyResources(upgradeTest.ResourceDirectories, time.Minute*45),
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
			upgrade.DeleteResources(upgradeTest.ResourceDirectories, time.Minute*45),
		).
		WithTeardown(
			"delete provider",
			upgrade.DeleteProvider(upgradeTest.ProviderName),
		)
	testenv.Test(t, upgradeFeature.Feature())
}
