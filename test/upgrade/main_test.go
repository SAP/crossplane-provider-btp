//go:build upgrade

package upgrade

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/images"
	"github.com/crossplane-contrib/xp-testing/pkg/setup"
	"github.com/sap/crossplane-provider-btp/test"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/support/kind"
)

const (
	crossplaneVersion = "1.20.1"

	providerName = "provider-btp"

	namespacePrefix = "btp-upgrade-test-"

	localTagName = "local"

	cisSecretName         = "cis-provider-secret"
	serviceUserSecretName = "sa-provider-secret"

	globalAccountEnvVar = "GLOBAL_ACCOUNT"
	cliServerUrlEnvVar  = "CLI_SERVER_URL"
	uutImagesEnvVar     = "UUT_IMAGES"

	fromTagEnvVar                  = "UPGRADE_TEST_FROM_TAG"
	toTagEnvVar                    = "UPGRADE_TEST_TO_TAG"
	fromProviderRepositoryEnvVar   = "UPGRADE_TEST_FROM_PROVIDER_REPOSITORY"
	toProviderRepositoryEnvVar     = "UPGRADE_TEST_TO_PROVIDER_REPOSITORY"
	fromControllerRepositoryEnvVar = "UPGRADE_TEST_FROM_CONTROLLER_REPOSITORY"
	toControllerRepositoryEnvVar   = "UPGRADE_TEST_TO_CONTROLLER_REPOSITORY"

	verifyTimeoutEnvVar = "UPGRADE_TEST_VERIFY_TIMEOUT"
	waitForPauseEnvVar  = "UPGRADE_TEST_WAIT_FOR_PAUSE"

	defaultProviderRepository   = "ghcr.io/sap/crossplane-provider-btp/crossplane/provider-btp"
	defaultControllerRepository = "ghcr.io/sap/crossplane-provider-btp/crossplane/provider-btp-controller"

	defaultVerifyTimeoutMins = 30
	defaultWaitForPauseMins  = 1
)

// upgradeCRsGeneratedPathEnv names the env var that points at the rendered
// upgrade-test fixture tree (envsubst-substituted). `make generate-upgrade-test-crs`
// renders templates from UPGRADE_TEST_CRS_PATH into this directory; the var
// is exported at the top of the Makefile, so it is already in the test
// binary's environment when invoked via `make upgrade-test`. Falls back to
// the in-tree templates dir so raw `go test -tags=upgrade` keeps working
// for inspection (values won't be substituted in that case).
const upgradeCRsGeneratedPathEnv = "UPGRADE_TEST_CRS_GENERATED_PATH"

// upgradeCRsPath resolves a fixture path relative to the upgrade-test CRs
// directory. Pass the relative path you would have hard-coded under
// `./testdata/`, e.g.
//
//	upgradeCRsPath("baseCRs")                       -> "<generated>/baseCRs"
//	upgradeCRsPath("customCRs/directoryExternalName") -> "<generated>/customCRs/directoryExternalName"
func upgradeCRsPath(rel string) string {
	base := os.Getenv(upgradeCRsGeneratedPathEnv)
	if base == "" {
		base = "./testdata"
	}
	return base + "/" + rel
}

// resourceDirectoryRoot returns the rendered baseCRs directory. Defined as a
// function (not a const) because the rendered root depends on the
// UPGRADE_TEST_CRS_GENERATED_PATH env var, which is only set when the test
// runs under `make upgrade-test`.
func resourceDirectoryRoot() string { return upgradeCRsPath("baseCRs") }

var (
	globalVerifyTimeout time.Duration
	globalWaitForPause  time.Duration

	bindingSecretData map[string]string
	userSecretData    map[string]string
	globalAccount     string
	cliServerUrl      string

	fromProviderRepository   string
	toProviderRepository     string
	fromControllerRepository string
	toControllerRepository   string

	kindClusterName string
	namespace       string
	testenv         env.Environment
)

func TestMain(m *testing.M) {
	testenv = env.New()
	var verbosity = 4
	test.SetupLogging(verbosity)

	namespace = envconf.RandomName(namespacePrefix, 16)

	// Load ProviderConfig secrets
	bindingSecretData = test.GetBindingSecretOrPanic()
	userSecretData = test.GetUserSecretOrPanic()
	globalAccount = envvar.GetOrPanic(globalAccountEnvVar)
	cliServerUrl = envvar.GetOrPanic(cliServerUrlEnvVar)

	// Load repositories
	fromProviderRepository = test.GetEnv(fromProviderRepositoryEnvVar, defaultProviderRepository)
	toProviderRepository = test.GetEnv(toProviderRepositoryEnvVar, defaultProviderRepository)
	fromControllerRepository = test.GetEnv(fromControllerRepositoryEnvVar, defaultControllerRepository)
	toControllerRepository = test.GetEnv(toControllerRepositoryEnvVar, defaultControllerRepository)

	// Load timeouts
	globalVerifyTimeout = test.LoadDurationMins(verifyTimeoutEnvVar, defaultVerifyTimeoutMins)
	globalWaitForPause = test.LoadDurationMins(waitForPauseEnvVar, defaultWaitForPauseMins)

	// Setup cluster
	fromTag, toTag := LoadUpgradeTags()

	fromProviderPackage, _, fromControllerPackage, _ := test.LoadUpgradePackages(
		fromTag, toTag,
		fromProviderRepository, toProviderRepository,
		fromControllerRepository, toControllerRepository,
		uutImagesEnvVar, localTagName,
		true,
	)

	setupClusterWithCrossplane(fromTag, fromProviderPackage, fromControllerPackage)

	os.Exit(testenv.Run(m))
}

// setupClusterWithCrossplane sets up a kind cluster with Crossplane and the specified provider version.
// It does not create a ProviderConfig, as this is done in the individual tests.
// Setting up the provider is technically not necessary for upgrade tests, but that's what xp-testing's setup does.
func setupClusterWithCrossplane(fromTag, providerPackage, controllerPackage string) {
	deploymentRuntimeConfig := test.DeploymentRuntimeConfig(providerName, fromTag)

	cfg := setup.ClusterSetup{
		ProviderName: providerName,
		Images: images.ProviderImages{
			Package:         providerPackage,
			ControllerImage: &controllerPackage,
		},
		CrossplaneSetup: setup.CrossplaneSetup{
			Version:  crossplaneVersion,
			Registry: setup.DockerRegistry,
		},
		DeploymentRuntimeConfig: &deploymentRuntimeConfig,
	}

	cfg.PostCreate(func(clusterName string) env.Func {
		return func(ctx context.Context, config *envconf.Config) (context.Context, error) {
			kindClusterName = clusterName
			klog.V(4).Infof("Upgrade cluster %s has been created", clusterName)
			return ctx, nil
		}
	})

	_ = cfg.Configure(testenv, &kind.Cluster{})

	testenv.Setup(
		test.ApplySecretInCrossplaneNamespace(cisSecretName, bindingSecretData),
		test.ApplySecretInCrossplaneNamespace(serviceUserSecretName, userSecretData),
	)
}

func LoadUpgradeTags() (string, string) {
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
