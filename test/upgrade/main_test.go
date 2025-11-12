//go:build upgrade

package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/vendored"
	"github.com/crossplane-contrib/xp-testing/pkg/xpenvfuncs"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	meta_api "github.com/sap/crossplane-provider-btp/apis"
	apiV1Alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/vladimirvivien/gexe"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/support/kind"

	"sigs.k8s.io/e2e-framework/pkg/env"

	"github.com/crossplane-contrib/xp-testing/pkg/images"
	"github.com/crossplane-contrib/xp-testing/pkg/logging"
	"github.com/crossplane-contrib/xp-testing/pkg/setup"
)

const (
	providerName              = "provider-btp"
	resourceDirectoryRoot     = "../e2e/testdata/crs"
	packageBasePath           = "ghcr.io/sap/crossplane-provider-btp/crossplane/provider-btp"
	controllerPackageBasePath = "ghcr.io/sap/crossplane-provider-btp/crossplane/provider-btp-controller"
	cisSecretName             = "cis-provider-secret"
	serviceUserSecretName     = "sa-provider-secret"
)

var (
	testenv             env.Environment
	kindClusterName     string
	resourceDirectories []string
)

var (
	fromTag     string
	toTag       string
	fromPackage string
	toPackage   string
)

func TestMain(m *testing.M) {
	var verbosity = 4
	setupLogging(verbosity)

	namespace := envconf.RandomName("test-ns", 16)

	SetupClusterWithCrossplane(namespace)

	os.Exit(testenv.Run(m))
}

func SetupClusterWithCrossplane(namespace string) {
	testenv = env.New()

	bindingSecretData := getBindingSecretOrPanic()
	userSecretData := getUserSecretOrPanic()
	globalAccount := envvar.GetOrPanic("GLOBAL_ACCOUNT")
	cliServerUrl := envvar.GetOrPanic("CLI_SERVER_URL")

	fromTag, toTag = loadPackageTags()

	loadResourceDirectories()
	klog.V(4).Infof("found resource directories: %s", resourceDirectories)

	fromPackage = fmt.Sprintf("%s:%s", packageBasePath, fromTag)
	toPackage = fmt.Sprintf("%s:%s", packageBasePath, toTag)
	fromControllerPackage := fmt.Sprintf("%s:%s", controllerPackageBasePath, fromTag)
	toControllerPackage := fmt.Sprintf("%s:%s", controllerPackageBasePath, toTag)

	mustPullImage(fromPackage)
	mustPullImage(toPackage)
	mustPullImage(fromControllerPackage)
	mustPullImage(toControllerPackage)

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
		ProviderName: "provider-btp",
		Images: images.ProviderImages{
			Package:         fromPackage,
			ControllerImage: &fromControllerPackage,
		},
		CrossplaneSetup: setup.CrossplaneSetup{
			Version:  "1.20.1",
			Registry: setup.DockerRegistry,
		},
		DeploymentRuntimeConfig: &deploymentRuntimeConfig,
	}

	cfg.PostCreate(func(clusterName string) env.Func {
		kindClusterName = clusterName
		return func(ctx context.Context, config *envconf.Config) (context.Context, error) {
			klog.V(4).Infof("upgrade cluster %s has been created", clusterName)
			return ctx, nil
		}
	})

	_ = cfg.Configure(testenv, &kind.Cluster{})

	testenv.Setup(
		ApplySecretInCrossplaneNamespace(cisSecretName, bindingSecretData),
		ApplySecretInCrossplaneNamespace(serviceUserSecretName, userSecretData),
		createProviderConfigFn(namespace, globalAccount, cliServerUrl),
	)
}

func loadPackageTags() (string, string) {
	fromTagVar := os.Getenv("UPGRADE_TEST_FROM_TAG")
	if fromTagVar == "" {
		panic("UPGRADE_TEST_FROM_TAG environment variable is required")
	}

	toTagVar := os.Getenv("UPGRADE_TEST_TO_TAG")
	if toTagVar == "" {
		panic("UPGRADE_TEST_TO_TAG environment variable is required")
	}

	return fromTagVar, toTagVar
}

func loadDirectoriesWithYAMLFiles(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read resource files from %s: %w", path, err)
	}

	var directories []string
	containsYAMLFile := false

	for _, entry := range entries {
		if entry.IsDir() {
			subEntries, err := loadDirectoriesWithYAMLFiles(filepath.Join(path, entry.Name()))
			if err != nil {
				return nil, err
			}

			directories = append(directories, subEntries...)
		} else if strings.HasSuffix(entry.Name(), ".yaml") {
			containsYAMLFile = true
		}
	}

	if containsYAMLFile {
		directories = append(directories, path)
	}

	return directories, nil
}

func loadResourceDirectories() {
	directories, err := loadDirectoriesWithYAMLFiles(resourceDirectoryRoot)
	if err != nil {
		panic(fmt.Errorf("failed to read resource directories from %s: %w", resourceDirectoryRoot, err))
	}

	resourceDirectories = directories
}

func mustPullImage(image string) {
	klog.Info("Pulling ", image)
	runner := gexe.New()
	p := runner.RunProc(fmt.Sprintf("docker pull %s", image))
	klog.V(4).Info(p.Out())
	if p.Err() != nil {
		panic(fmt.Errorf("docker pull %v failed: %w: %s", image, p.Err(), p.Result()))
	}
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
							Name:      serviceUserSecretName,
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
							Name:      cisSecretName,
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

func setupLogging(verbosity int) {
	logging.EnableVerboseLogging(&verbosity)
	zl := zap.New(zap.UseDevMode(true))
	ctrl.SetLogger(zl)
}
