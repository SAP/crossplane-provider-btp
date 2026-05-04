//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/images"
	"github.com/crossplane-contrib/xp-testing/pkg/setup"
	"github.com/crossplane-contrib/xp-testing/pkg/vendored"
	testutil "github.com/sap/crossplane-provider-btp/test"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/third_party/kind"
)

var (
	CIS_SECRET_NAME              = "cis-provider-secret"
	SERVICE_USER_SECRET_NAME     = "sa-provider-secret"
	TECHNICAL_USER_EMAIL_ENV_KEY = "TECHNICAL_USER_EMAIL"
	SECONDARY_DIRC_ADMIN_ENV_KEY = "SECOND_DIRECTORY_ADMIN_EMAIL"

	UUT_BUILD_ID_KEY = "BUILD_ID"
	UUT_IMAGES_KEY   = "UUT_IMAGES"
)

var (
	testenv  env.Environment
	BUILD_ID string
)

func TestMain(m *testing.M) {
	var verbosity = 4
	testutil.SetupLogging(verbosity)

	namespace := envconf.RandomName("test-ns", 16)

	SetupClusterWithCrossplane(namespace)

	os.Exit(testenv.Run(m))
}

func SetupClusterWithCrossplane(namespace string) {
	// e.g. pr-16-3... defaults to empty string if not set
	BUILD_ID = envvar.Get(UUT_BUILD_ID_KEY)

	testenv = env.New()

	bindingSecretData := testutil.GetBindingSecretOrPanic()
	userSecretData := testutil.GetUserSecretOrPanic()
	globalAccount := envvar.GetOrPanic("GLOBAL_ACCOUNT")
	cliServerUrl := envvar.GetOrPanic("CLI_SERVER_URL")

	// Setup uses pre-defined funcs to create kind cluster
	// and create a namespace for the environment.
	// When E2E_REUSE_CLUSTER is set (via local-deploy), the provider is already
	// installed and xp-testing skips installation. Otherwise (e.g. long e2e tests),
	// we load images from UUT_IMAGES and install the provider ourselves.

	deploymentRuntimeConfig := vendored.DeploymentRuntimeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "btp-provider-runtime-config",
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

	cfg := setup.ClusterSetup{
		ProviderName: "btp-account",
		Images:       loadProviderImages(),
		CrossplaneSetup: setup.CrossplaneSetup{
			Version:  "1.20.1",
			Registry: setup.DockerRegistry,
		},
		DeploymentRuntimeConfig: &deploymentRuntimeConfig,
	}

	cfg.Configure(testenv, &kind.Cluster{})

	testenv.Setup(
		envfuncs.CreateNamespace(namespace),
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			cfg.WithNamespace(namespace)
			return ctx, nil
		},
		testutil.ApplySecretInCrossplaneNamespace(CIS_SECRET_NAME, bindingSecretData),
		testutil.ApplySecretInCrossplaneNamespace(SERVICE_USER_SECRET_NAME, userSecretData),
		testutil.CreateProviderConfigEnvFn(namespace, globalAccount, cliServerUrl, CIS_SECRET_NAME, SERVICE_USER_SECRET_NAME),
	)
}

// loadProviderImages loads provider images from UUT_IMAGES env var if set.
// When E2E_REUSE_CLUSTER is used, the provider is already installed so images
// are not needed and this returns an empty ProviderImages.
func loadProviderImages() images.ProviderImages {
	uutImages := os.Getenv(UUT_IMAGES_KEY)
	if uutImages == "" {
		return images.ProviderImages{}
	}

	uutConfig, uutController := testutil.GetImagesFromJsonOrPanic(uutImages)
	return images.ProviderImages{
		Package:         uutConfig,
		ControllerImage: &uutController,
	}
}
