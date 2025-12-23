//go:build upgrade

package upgrade

import (
	"context"
	"fmt"
	"testing"

	"github.com/crossplane-contrib/xp-testing/pkg/upgrade"
	"github.com/sap/crossplane-provider-btp/test"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestUpgradeProvider(t *testing.T) {
	klog.V(2).Infof("Starting upgrade test from %s to %s", fromTag, toTag)
	klog.V(2).Infof("Testing resources in directories: %v", resourceDirectories)

	// TODO: Make sure there are logs for the full package strings

	upgradeTest := upgrade.UpgradeTest{
		ProviderName:        providerName,
		ClusterName:         kindClusterName,
		FromProviderPackage: fromProviderPackage,
		ToProviderPackage:   toProviderPackage,
		ResourceDirectories: resourceDirectories,
	}

	upgradeFeature := features.New(fmt.Sprintf("Upgrade %s from %s to %s", providerName, fromTag, toTag)).
		WithSetup(
			"Install provider with version "+fromTag,
			upgrade.ApplyProvider(upgradeTest.ClusterName, upgradeTest.FromProviderInstallOptions()),
		).
		WithSetup(
			"Import resources in "+resourceDirectoryRoot,
			upgrade.ImportResources(upgradeTest.ResourceDirectories),
		).
		Assess(
			"Verify resources before upgrade",
			upgrade.VerifyResources(upgradeTest.ResourceDirectories, verifyTimeout),
		).
		Assess("Upgrade provider to version "+toTag, upgrade.UpgradeProvider(upgrade.UpgradeProviderOptions{
			ClusterName: upgradeTest.ClusterName,
			ProviderOptions: test.InstallProviderOptionsWithController(
				upgradeTest.ToProviderInstallOptions(),
				toControllerPackage,
			),
			ResourceDirectories: upgradeTest.ResourceDirectories,
			WaitForPause:        waitForPause,
		})).
		Assess(
			"Verify resources after upgrade",
			upgrade.VerifyResources(upgradeTest.ResourceDirectories, verifyTimeout),
		).
		WithTeardown(
			"Delete resources",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				err := test.DeleteResourcesFromDirsGracefully(
					ctx,
					cfg,
					resourceDirectories,
					wait.WithTimeout(verifyTimeout),
				)
				if err != nil {
					t.Logf("failed to clean up resources: %v", err)
				}

				return ctx
			},
		).
		WithTeardown(
			"Delete provider",
			upgrade.DeleteProvider(upgradeTest.ProviderName),
		)
	testenv.Test(t, upgradeFeature.Feature())
}
