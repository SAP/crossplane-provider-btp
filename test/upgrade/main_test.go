//go:build upgrade

package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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
	providerName              = "provider-btp"
	resourceDirectoryRoot     = "../e2e/testdata/crs"
	packageBasePath           = "ghcr.io/sap/crossplane-provider-btp/crossplane/provider-btp"
	controllerPackageBasePath = "ghcr.io/sap/crossplane-provider-btp/crossplane/provider-btp-controller"
	cisSecretName             = "cis-provider-secret"
	serviceUserSecretName     = "sa-provider-secret"
)

var (
	testenv                   env.Environment
	kindClusterName           string
	resourceDirectories       []string
	ignoreResourceDirectories = []string{
		"../e2e/testdata/crs/entitlement_cf", // not used in e2e tests
	}
)

var (
	fromTag     string
	toTag       string
	fromPackage string
	toPackage   string
)

func TestMain(m *testing.M) {
	var verbosity = 4
	testutil.SetupLogging(verbosity)

	namespace := envconf.RandomName("test-ns", 16)

	SetupClusterWithCrossplane(namespace)

	os.Exit(testenv.Run(m))
}

func SetupClusterWithCrossplane(namespace string) {
	testenv = env.New()

	bindingSecretData := testutil.GetBindingSecretOrPanic()
	userSecretData := testutil.GetUserSecretOrPanic()
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
		testutil.ApplySecretInCrossplaneNamespace(cisSecretName, bindingSecretData),
		testutil.ApplySecretInCrossplaneNamespace(serviceUserSecretName, userSecretData),
		testutil.CreateProviderConfigFn(namespace, globalAccount, cliServerUrl, cisSecretName, serviceUserSecretName),
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
			if !slices.Contains(ignoreResourceDirectories, filepath.Join(path, entry.Name())) {
				subEntries, err := loadDirectoriesWithYAMLFiles(filepath.Join(path, entry.Name()))
				if err != nil {
					return nil, err
				}

				directories = append(directories, subEntries...)
			}
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
