//go:build upgrade

package upgrade

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/images"
	"github.com/crossplane-contrib/xp-testing/pkg/setup"
	"github.com/crossplane-contrib/xp-testing/pkg/vendored"
	testutil "github.com/sap/crossplane-provider-btp/test"
	"github.com/vladimirvivien/gexe"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	testenv             env.Environment
	kindClusterName     string
	resourceDirectories []string
	// Add any directories to ignore here, e.g.: ./testdata/baseCRs/ignore-this-dir
	ignoreResourceDirectories []string

	verifyTimeout time.Duration
	waitForPause  time.Duration
)

var (
	fromTag               string
	toTag                 string
	fromProviderPackage   string
	toProviderPackage     string
	fromControllerPackage string
	toControllerPackage   string
)

func TestMain(m *testing.M) {
	var verbosity = 4
	testutil.SetupLogging(verbosity)

	namespace := envconf.RandomName(namespacePrefix, 16)

	SetupClusterWithCrossplane(namespace)

	os.Exit(testenv.Run(m))
}

func SetupClusterWithCrossplane(namespace string) {
	testenv = env.New()

	bindingSecretData := testutil.GetBindingSecretOrPanic()
	userSecretData := testutil.GetUserSecretOrPanic()
	globalAccount := envvar.GetOrPanic(globalAccountEnvVar)
	cliServerUrl := envvar.GetOrPanic(cliServerUrlEnvVar)

	fromTag, toTag = loadTags()
	fromProviderRepository, toProviderRepository, fromControllerRepository, toControllerRepository := loadRepositories()

	verifyTimeout = loadDurationMins(verifyTimeoutEnvVar, defaultVerifyTimeoutMins)
	waitForPause = loadDurationMins(waitForPauseEnvVar, defaultWaitForPauseMins)

	resourceDirectories = loadResourceDirectories()
	klog.V(4).Infof("Found the following resource directories: %s", resourceDirectories)

	resolvePackages(
		fromTag,
		toTag,
		fromProviderRepository,
		toProviderRepository,
		fromControllerRepository,
		toControllerRepository,
	)

	deploymentRuntimeConfig := getDeploymentRuntimeConfig(providerName)

	cfg := setup.ClusterSetup{
		ProviderName: providerName,
		Images: images.ProviderImages{
			Package:         fromProviderPackage,
			ControllerImage: &fromControllerPackage,
		},
		CrossplaneSetup: setup.CrossplaneSetup{
			Version:  crossplaneVersion,
			Registry: setup.DockerRegistry,
		},
		DeploymentRuntimeConfig: &deploymentRuntimeConfig,
	}

	cfg.PostCreate(func(clusterName string) env.Func {
		kindClusterName = clusterName
		return func(ctx context.Context, config *envconf.Config) (context.Context, error) {
			klog.V(4).Infof("Upgrade cluster %s has been created", clusterName)
			return ctx, nil
		}
	})

	_ = cfg.Configure(testenv, &kind.Cluster{})

	testenv.Setup(
		testutil.ApplySecretInCrossplaneNamespace(cisSecretName, bindingSecretData),
		testutil.ApplySecretInCrossplaneNamespace(serviceUserSecretName, userSecretData),
		testutil.CreateProviderConfigFn(namespace, globalAccount, cliServerUrl, cisSecretName, serviceUserSecretName),
	)
}

func resolvePackages(
	fromTag, toTag string,
	fromProviderRepository, toProviderRepository, fromControllerRepository, toControllerRepository string,
) {
	isLocalFromTag := fromTag == localTagName
	isLocalToTag := toTag == localTagName

	// If either tag is local, parse `UUT_IMAGES` once.
	if isLocalFromTag || isLocalToTag {
		uutImages := os.Getenv(uutImagesEnvVar)
		if uutImages == "" {
			panic(uutImagesEnvVar + " environment variable is required when FROM_TAG or TO_TAG is set to \"local\"")
		}

		localProviderPackage, localControllerPackage := testutil.GetImagesFromJsonOrPanic(uutImages)
		localTag := strings.Split(localProviderPackage, ":")[1]

		if isLocalFromTag {
			fromTag = localTag
			fromProviderPackage = localProviderPackage
			fromControllerPackage = localControllerPackage
		}

		if isLocalToTag {
			toTag = localTag
			toProviderPackage = localProviderPackage
			toControllerPackage = localControllerPackage
		}
	}

	if !isLocalFromTag {
		fromProviderPackage = fmt.Sprintf("%s:%s", fromProviderRepository, fromTag)
		fromControllerPackage = fmt.Sprintf("%s:%s", fromControllerRepository, fromTag)

		mustPullImage(fromProviderPackage)
		mustPullImage(fromControllerPackage)
	}

	if !isLocalToTag {
		toProviderPackage = fmt.Sprintf("%s:%s", toProviderRepository, toTag)
		toControllerPackage = fmt.Sprintf("%s:%s", toControllerRepository, toTag)

		mustPullImage(toProviderPackage)
		mustPullImage(toControllerPackage)
	}
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

func loadRepositories() (string, string, string, string) {
	fromProviderRepository := getEnv(fromProviderRepositoryEnvVar, defaultProviderRepository)
	toProviderRepository := getEnv(toProviderRepositoryEnvVar, defaultProviderRepository)
	fromControllerRepository := getEnv(fromControllerRepositoryEnvVar, defaultControllerRepository)
	toControllerRepository := getEnv(toControllerRepositoryEnvVar, defaultControllerRepository)

	return fromProviderRepository, toProviderRepository, fromControllerRepository, toControllerRepository
}

func loadResourceDirectories() []string {
	directories, err := testutil.LoadDirectoriesWithYAMLFiles(resourceDirectoryRoot, ignoreResourceDirectories)
	if err != nil {
		panic(fmt.Errorf("failed to read resource directories from %s: %w", resourceDirectoryRoot, err))
	}

	return directories
}

func loadDurationMins(envVar string, defaultValue int) time.Duration {
	durationStr := os.Getenv(envVar)
	if durationStr == "" {
		klog.V(4).Infof("%s not found, defaulting to %d minutes", envVar, defaultValue)
		return time.Duration(defaultValue) * time.Minute
	}

	durationMin, err := strconv.Atoi(durationStr)
	if err != nil {
		klog.Warningf("%s value \"%s\" is invalid, defaulting to %d minutes", envVar, durationStr, defaultValue)
		return time.Duration(defaultValue) * time.Minute
	}

	if durationMin <= 0 {
		klog.Warningf(
			"%s value \"%d\" is invalid (must be > 0), defaulting to %d minutes",
			envVar,
			durationMin,
			defaultValue,
		)
		return time.Duration(defaultValue) * time.Minute
	}

	klog.V(4).Infof("Using %s of %d minutes", envVar, durationMin)
	return time.Duration(durationMin) * time.Minute
}

func mustPullImage(image string) {
	klog.V(4).Info("Pulling ", image)
	runner := gexe.New()
	p := runner.RunProc(fmt.Sprintf("docker pull %s", image))
	if p.Err() != nil {
		panic(fmt.Errorf("docker pull %v failed: %w: %s", image, p.Err(), p.Result()))
	}
	klog.V(4).Info("Pulled ", image)
}

func getDeploymentRuntimeConfig(namePrefix string) vendored.DeploymentRuntimeConfig {
	return vendored.DeploymentRuntimeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: namePrefix + "-runtime-config",
		},
		Spec: vendored.DeploymentRuntimeConfigSpec{
			DeploymentTemplate: &vendored.DeploymentTemplate{
				Spec: &appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "package-runtime",
									Args: []string{"--debug", "--sync=10s"},
								},
							},
						},
					},
				},
			},
		},
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return fallback
}
