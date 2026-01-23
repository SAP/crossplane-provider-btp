//go:build upgrade

package upgrade

import (
	"os"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/sap/crossplane-provider-btp/test"
)

const (
	crossplaneVersion = "1.20.1"

	providerName = "provider-btp"

	namespacePrefix = "btp-upgrade-test-"

	localTagName = "local"

	resourceDirectoryRoot = "./testdata/baseCRs"

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
)

func TestMain(m *testing.M) {
	var verbosity = 4
	test.SetupLogging(verbosity)

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

	os.Exit(m.Run())
}
