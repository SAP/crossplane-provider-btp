//go:build e2e

package e2e

import (
	"context"
	"os"
	"strings"
	"testing"

	"encoding/json"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/images"
	"github.com/crossplane-contrib/xp-testing/pkg/logging"
	"github.com/crossplane-contrib/xp-testing/pkg/setup"
	"github.com/crossplane-contrib/xp-testing/pkg/vendored"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"

	"github.com/crossplane-contrib/xp-testing/pkg/xpenvfuncs"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	meta_api "github.com/sap/crossplane-provider-btp/apis"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/third_party/kind"

	"github.com/pkg/errors"
	apiV1Alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/vladimirvivien/gexe"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	UUT_IMAGES_KEY     = "UUT_IMAGES"
	UUT_CONFIG_KEY     = "crossplane/provider-btp"
	UUT_CONTROLLER_KEY = "crossplane/provider-btp-controller"

	CIS_SECRET_NAME          = "cis-provider-secret"
	SERVICE_USER_SECRET_NAME = "sa-provider-secret"

	UUT_BUILD_ID_KEY = "BUILD_ID"
)

var (
	testenv  env.Environment
	BUILD_ID string
)

func TestMain(m *testing.M) {
	var verbosity = 4
	setupLogging(verbosity)

	namespace := envconf.RandomName("test-ns", 16)

	SetupClusterWithCrossplane(namespace)

	os.Exit(testenv.Run(m))
}

func SetupClusterWithCrossplane(namespace string) {
	// e.g. pr-16-3... defaults to empty string if not set
	BUILD_ID = envvar.Get(UUT_BUILD_ID_KEY)

	uutImages := envvar.GetOrPanic(UUT_IMAGES_KEY)
	uutConfig, uutController := GetImagesFromJsonOrPanic(uutImages)

	testenv = env.New()

	bindingSecretData := getBindingSecretOrPanic()
	userSecretData := getUserSecretOrPanic()
	globalAccount := envvar.GetOrPanic("GLOBAL_ACCOUNT")
	cliServerUrl := envvar.GetOrPanic("CLI_SERVER_URL")

	// Setup uses pre-defined funcs to create kind cluster
	// and create a namespace for the environment

	deploymentRuntimeConfig := vendored.DeploymentRuntimeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "btp-provider-runtime-config",
		},
		Spec: vendored.DeploymentRuntimeConfigSpec{
			DeploymentTemplate: &vendored.DeploymentTemplate{
				Spec: &appsv1.DeploymentSpec{
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
		Images: images.ProviderImages{
			Package:         uutConfig,
			ControllerImage: &uutController,
		},
		CrossplaneSetup: setup.CrossplaneSetup{
			Version: "1.18.2",
		},
		DeploymentRuntimeConfig: &deploymentRuntimeConfig,
	}

	cfg.Configure(testenv, &kind.Cluster{})

	testenv.Setup(
		ApplySecretInCrossplaneNamespace(CIS_SECRET_NAME, bindingSecretData),
		ApplySecretInCrossplaneNamespace(SERVICE_USER_SECRET_NAME, userSecretData),
		createProviderConfigFn(namespace, globalAccount, cliServerUrl),
	)
}

func checkEnvVarExists(existsKey string) bool {
	v := os.Getenv(existsKey)

	if v == "1" {
		return true
	}

	return false
}

func createProviderConfigFn(namespace string, globalAccount string, cliServerUrl string) func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		r, _ := res.New(cfg.Client().RESTConfig())
		_ = meta_api.AddToScheme(r.GetScheme())

		obj := providerConfig(namespace, globalAccount, cliServerUrl)
		err := r.Create(ctx, obj)
		if kubeErrors.IsAlreadyExists(err) {
			return ctx, r.Update(ctx, obj)
		}
		return ctx, err
	}
}

func providerConfig(namespace string, globalAccount string, cliServerUrl string) *apiV1Alpha1.ProviderConfig {
	return &apiV1Alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: namespace,
		},
		Spec: apiV1Alpha1.ProviderConfigSpec{
			ServiceAccountSecret: apiV1Alpha1.ProviderCredentials{
				Source: "Secret",
				CommonCredentialSelectors: v1.CommonCredentialSelectors{
					SecretRef: &v1.SecretKeySelector{
						SecretReference: v1.SecretReference{
							Name:      SERVICE_USER_SECRET_NAME,
							Namespace: "crossplane-system",
						},
						Key: "credentials",
					},
				},
			},
			CISSecret: apiV1Alpha1.ProviderCredentials{
				Source: "Secret",
				CommonCredentialSelectors: v1.CommonCredentialSelectors{
					SecretRef: &v1.SecretKeySelector{
						SecretReference: v1.SecretReference{
							Name:      CIS_SECRET_NAME,
							Namespace: "crossplane-system",
						},
						Key: "data",
					},
				},
			},
			GlobalAccount: globalAccount,
			CliServerUrl:  cliServerUrl,
		},
		Status: apiV1Alpha1.ProviderConfigStatus{},
	}
}

func getBindingSecretOrPanic() map[string]string {

	binding := envvar.GetOrPanic("CIS_CENTRAL_BINDING")

	bindingSecret := map[string]string{
		"data": binding,
	}

	return bindingSecret
}

func getUserSecretOrPanic() map[string]string {

	user := envvar.GetOrPanic("BTP_TECHNICAL_USER")

	userSecret := map[string]string{
		"credentials": user,
	}

	return userSecret
}

func clusterExists(name string) bool {
	e := gexe.New()
	clusters := e.Run("kind get clusters")
	for _, c := range strings.Split(clusters, "\n") {
		if c == name {
			return true
		}
	}
	return false
}

func GetImagesFromJsonOrPanic(imagesJson string) (string, string) {

	imageMap := map[string]string{}

	err := json.Unmarshal([]byte(imagesJson), &imageMap)

	if err != nil {
		panic(errors.Wrap(err, "failed to unmarshal json from UUT_IMAGE"))
	}

	uutConfig := imageMap[UUT_CONFIG_KEY]
	uutController := imageMap[UUT_CONTROLLER_KEY]

	return uutConfig, uutController
}

func getUserNameFromSecretOrError(t *testing.T) string {
	secretData := getUserSecretOrPanic()
	secretJson := map[string]string{}
	err := json.Unmarshal([]byte(secretData["credentials"]), &secretJson)
	if err != nil {
		t.Fatal("error while retrieving technical user email")
	}
	return secretJson["email"]
}

func setupLogging(verbosity int) {
	logging.EnableVerboseLogging(&verbosity)
	zl := zap.New(zap.UseDevMode(true))
	ctrl.SetLogger(zl)
}

func ApplySecretInCrossplaneNamespace(name string, data map[string]string) env.Func {
	return xpenvfuncs.Compose(
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			r, err := resources.New(cfg.Client().RESTConfig())

			if err != nil {
				klog.Error(err)
				return ctx, err
			}

			secret := xpenvfuncs.SimpleSecret(name, xpenvfuncs.CrossplaneNamespace, data)

			if err := r.Create(ctx, secret); err != nil {
				if kubeErrors.IsAlreadyExists(err) {
					return ctx, r.Update(ctx, secret)
				}
				klog.Error(err)
				return ctx, err
			}

			return ctx, nil
		},
	)
}
