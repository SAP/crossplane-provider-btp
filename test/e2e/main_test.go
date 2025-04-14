//go:build e2e

package e2e

import (
	"context"
	"os"
	"strings"
	"testing"

	"encoding/json"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/logging"
	"github.com/crossplane-contrib/xp-testing/pkg/setup"
	"github.com/crossplane-contrib/xp-testing/pkg/vendored"
	"github.com/crossplane-contrib/xp-testing/pkg/xpenvfuncs"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"

	meta_api "github.com/sap/crossplane-provider-btp/apis"

	"github.com/pkg/errors"
	"github.com/vladimirvivien/gexe"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"

	apiV1Alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
)

var (
	UUT_IMAGES_KEY     = "UUT_IMAGES"
	UUT_CONFIG_KEY     = "package"
	UUT_CONTROLLER_KEY = "controller"

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

	reuseCluster := checkEnvVarExists("TEST_REUSE_CLUSTER")

	kindClusterName := envvar.GetOrDefault("CLUSTER_NAME", envconf.RandomName("btpa-e2e", 10))

	firstSetup := true
	if reuseCluster && clusterExists(kindClusterName) {
		firstSetup = false
	}

	testenv = env.New()

	bindingSecretData := getBindingSecretOrPanic()
	userSecretData := getUserSecretOrPanic()
	globalAccount := envvar.GetOrPanic("GLOBAL_ACCOUNT")
	cliServerUrl := envvar.GetOrPanic("CLI_SERVER_URL")

	// Setup uses pre-defined funcs to create kind cluster
	// and create a namespace for the environment

	controllerConfig := vendored.ControllerConfig{
		Spec: vendored.ControllerConfigSpec{
			Args: []string{"--debug", "--sync=10s"},
		},
	}
	testenv.Setup(
		envfuncs.CreateKindCluster(kindClusterName),
		xpenvfuncs.Conditional(xpenvfuncs.InstallCrossplane(kindClusterName, xpenvfuncs.Registry(setup.DockerRegistry)), firstSetup),
		xpenvfuncs.Conditional(
			xpenvfuncs.InstallCrossplaneProvider(
				kindClusterName, xpenvfuncs.InstallCrossplaneProviderOptions{
					Name:             "btp-account",
					Package:          uutConfig,
					ControllerImage:  &uutController,
					ControllerConfig: &controllerConfig,
				},
			), firstSetup,
		),
		envfuncs.CreateNamespace(namespace),
		xpenvfuncs.Conditional(
			xpenvfuncs.ApplySecretInCrossplaneNamespace(CIS_SECRET_NAME, bindingSecretData),
			firstSetup,
		),
		xpenvfuncs.Conditional(
			xpenvfuncs.ApplySecretInCrossplaneNamespace(SERVICE_USER_SECRET_NAME, userSecretData),
			firstSetup,
		),
		xpenvfuncs.Conditional(
			createProviderConfigFn(namespace, globalAccount, cliServerUrl),
			firstSetup,
		),
	)

	// Finish uses pre-defined funcs to
	// remove namespace, then delete cluster
	testenv.Finish(
		envfuncs.DeleteNamespace(namespace),
		xpenvfuncs.Conditional(envfuncs.DestroyKindCluster(kindClusterName), !reuseCluster),
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

		err := r.Create(ctx, providerConfig(namespace, globalAccount, cliServerUrl))

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
