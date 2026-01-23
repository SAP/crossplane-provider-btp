//go:build upgrade

package upgrade

import (
	"fmt"
	"os"
	"testing"

	"github.com/sap/crossplane-provider-btp/test"
)

var (
	resourceDirectories []string
	// Add any directories to ignore here, e.g.: ./testdata/baseCRs/ignore-this-dir
	ignoreResourceDirectories []string
)

// Test_BaselineUpgradeProvider is the baseline upgrade test that verifies the provider can be
// successfully upgraded from one version to another while maintaining resource health.
//
// This test demonstrates the use of the CustomUpgradeTestBuilder framework with
// default baseline behavior. The test flow is:
//  1. Install provider at the "from" version
//  2. Import test resources from baseline directories
//  3. Verify all resources are healthy
//  4. Upgrade provider to the "to" version
//  5. Verify all resources remain healthy after upgrade
//  6. Clean up resources and provider
func Test_BaselineUpgradeProvider(t *testing.T) {
	resourceDirectories = loadResourceDirectories()

	fromTag, toTag := loadTags()

	upgradeTest := NewCustomUpgradeTest("baseline-upgrade-test").
		FromVersion(fromTag).
		ToVersion(toTag).
		WithResourceDirectories(resourceDirectories)

	upgradeTest.testenv.Test(t, upgradeTest.Feature())
}

func loadTags() (string, string) {
	fromTagVar := os.Getenv(fromTagEnvVar)
	if fromTagVar == "" {
		panic(fromTagEnvVar + " environment variable is required")
	}

	toTagVar := os.Getenv(toTagEnvVar)
	if toTagVar == "" {
		panic(toTagEnvVar + " environment variable is required")
	}

	return fromTagVar, toTagVar
}

func loadResourceDirectories() []string {
	directories, err := test.LoadDirectoriesWithYAMLFiles(resourceDirectoryRoot, ignoreResourceDirectories)
	if err != nil {
		panic(fmt.Errorf("failed to read resource directories from %s: %w", resourceDirectoryRoot, err))
	}

	return directories
}
