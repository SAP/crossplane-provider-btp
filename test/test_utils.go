package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/logging"
	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	"github.com/crossplane-contrib/xp-testing/pkg/vendored"
	"github.com/crossplane-contrib/xp-testing/pkg/xpenvfuncs"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metaApi "github.com/sap/crossplane-provider-btp/apis"
	apiV1Alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/vladimirvivien/gexe"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const (
	uutConfigKey     = "crossplane/provider-btp"
	uutControllerKey = "crossplane/provider-btp-controller"
)

type mockList struct {
	metav1.ListInterface
	runtime.Object
	Items []k8s.Object
}

func SetupLogging(verbosity int) {
	logging.EnableVerboseLogging(&verbosity)
	zl := zap.New(zap.UseDevMode(true))
	ctrl.SetLogger(zl)
}

func ApplySecretInCrossplaneNamespace(name string, data map[string]string) env.Func {
	return xpenvfuncs.Compose(
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			r, err := res.New(cfg.Client().RESTConfig())

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

func GetBindingSecretOrPanic() map[string]string {
	binding := envvar.GetOrPanic("CIS_CENTRAL_BINDING")

	bindingSecret := map[string]string{
		"data": binding,
	}

	return bindingSecret
}

func GetUserSecretOrPanic() map[string]string {
	user := envvar.GetOrPanic("BTP_TECHNICAL_USER")

	userSecret := map[string]string{
		"credentials": user,
	}

	return userSecret
}

// CreateProviderConfig contains the core logic for creating or updating a ProviderConfig.
// Returns an error if the operation fails.
func CreateProviderConfig(
	ctx context.Context,
	cfg *envconf.Config,
	namespace string,
	globalAccount string,
	cliServerUrl string,
	cisSecretName string,
	serviceUserSecretName string,
) error {
	r, err := res.New(cfg.Client().RESTConfig())
	if err != nil {
		return fmt.Errorf("failed to create resources client: %w", err)
	}

	err = metaApi.AddToScheme(r.GetScheme())
	if err != nil {
		return fmt.Errorf("failed to add scheme: %w", err)
	}

	obj := ProviderConfig(namespace, globalAccount, cliServerUrl, cisSecretName, serviceUserSecretName)
	err = r.Create(ctx, obj)
	if kubeErrors.IsAlreadyExists(err) {
		return r.Update(ctx, obj)
	}

	return err
}

// CreateProviderConfigFeatureFn returns a features.Func for use in feature.WithSetup.
func CreateProviderConfigFeatureFn(
	namespace string,
	globalAccount string,
	cliServerUrl string,
	cisSecretName string,
	serviceUserSecretName string,
) features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		err := CreateProviderConfig(ctx, cfg, namespace, globalAccount, cliServerUrl, cisSecretName, serviceUserSecretName)
		if err != nil {
			t.Errorf("failed to create ProviderConfig: %v", err)
		}
		return ctx
	}
}

// CreateProviderConfigEnvFn returns an env.Func for use in env.Setup.
func CreateProviderConfigEnvFn(
	namespace string,
	globalAccount string,
	cliServerUrl string,
	cisSecretName string,
	serviceUserSecretName string,
) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		err := CreateProviderConfig(ctx, cfg, namespace, globalAccount, cliServerUrl, cisSecretName, serviceUserSecretName)
		if err != nil {
			return ctx, fmt.Errorf("failed to create ProviderConfig: %w", err)
		}
		return ctx, nil
	}
}

func DeleteProviderConfigFn(
	namespace string,
	globalAccount string,
	cliServerUrl string,
	cisSecretName string,
	serviceUserSecretName string,
) features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		r, err := res.New(cfg.Client().RESTConfig())
		if err != nil {
			t.Errorf("failed to create resources client: %v", err)
			return ctx
		}

		err = metaApi.AddToScheme(r.GetScheme())
		if err != nil {
			t.Errorf("failed to add scheme: %v", err)
			return ctx
		}

		obj := ProviderConfig(namespace, globalAccount, cliServerUrl, cisSecretName, serviceUserSecretName)
		err = r.Delete(ctx, obj)
		if err != nil && !kubeErrors.IsNotFound(err) {
			t.Errorf("failed to delete ProviderConfig: %v", err)
		}

		return ctx
	}
}

func ProviderConfig(namespace string, globalAccount string, cliServerUrl string, cisSecretName string, serviceUserSecretName string) *apiV1Alpha1.ProviderConfig {
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

// DeleteResourcesFromDirsGracefully deletes previously imported resources from multiple directories, failing gracefully
func DeleteResourcesFromDirsGracefully(ctx context.Context, cfg *envconf.Config, manifestDirs []string, timeout wait.Option) error {
	klog.V(4).Info("Attempt to delete previously imported resources")
	r, _ := resources.GetResourcesWithRESTConfig(cfg)
	objects, err := GetObjectsToImport(ctx, cfg, manifestDirs)
	if err != nil {
		return err
	}

	for _, obj := range objects {
		delErr := r.Delete(ctx, obj)
		if delErr != nil && !kubeErrors.IsNotFound(delErr) {
			return delErr
		}
	}

	if err = wait.For(
		conditions.New(r).ResourcesDeleted(&mockList{Items: objects}),
		timeout,
	); err != nil {
		return err
	}

	return nil
}

func GetObjectsToImport(ctx context.Context, cfg *envconf.Config, dirs []string) ([]k8s.Object, error) {
	r := resClient(cfg)

	r.WithNamespace(cfg.Namespace())

	objects := make([]k8s.Object, 0)
	for _, dir := range dirs {
		err := decoder.DecodeEachFile(
			ctx, os.DirFS(dir), "*",
			func(ctx context.Context, obj k8s.Object) error {
				objects = append(objects, obj)
				return nil
			},
			decoder.MutateNamespace(cfg.Namespace()),
		)

		if err != nil {
			return nil, err
		}
	}

	return objects, nil
}

func resClient(cfg *envconf.Config) *res.Resources {
	r, _ := resources.GetResourcesWithRESTConfig(cfg)
	return r
}

func LoadDirectoriesWithYAMLFiles(path string, ignoreDirectories []string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read resource files from %s: %w", path, err)
	}

	var directories []string
	containsYAMLFile := false

	for _, entry := range entries {
		if entry.IsDir() {
			if !slices.Contains(ignoreDirectories, filepath.Join(path, entry.Name())) {
				subEntries, err := LoadDirectoriesWithYAMLFiles(filepath.Join(path, entry.Name()), ignoreDirectories)
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

func InstallProviderOptionsWithController(
	options xpenvfuncs.InstallCrossplaneProviderOptions, controllerPackage string,
) xpenvfuncs.InstallCrossplaneProviderOptions {
	options.ControllerImage = &controllerPackage
	return options
}

func GetImagesFromJsonOrPanic(imagesJson string) (string, string) {
	imageMap := map[string]string{}

	err := json.Unmarshal([]byte(imagesJson), &imageMap)

	if err != nil {
		panic(fmt.Errorf("failed to unmarshal images json: %w", err))
	}

	uutConfig := imageMap[uutConfigKey]
	uutController := imageMap[uutControllerKey]

	return uutConfig, uutController
}

// LoadUpgradePackages resolves provider and controller packages for upgrade tests.
//
// It handles both local and remote tags, pulling images as needed:
//   - "local" tag: Uses locally built images from the UUT_IMAGES env var
//   - Other tags: Constructs image URLs from repositories and optionally pulls them
//
// Returns: fromProviderPackage, toProviderPackage, fromControllerPackage, toControllerPackage
func LoadUpgradePackages(
	fromTag, toTag string,
	fromProviderRepository, toProviderRepository, fromControllerRepository, toControllerRepository string,
	uutImagesEnvVar, localTagName string,
	pullPackages bool,
) (string, string, string, string) {
	isLocalFromTag := fromTag == localTagName
	isLocalToTag := toTag == localTagName

	var fromProviderPackage, toProviderPackage, fromControllerPackage, toControllerPackage string

	// If either tag is local, parse UUT_IMAGES once.
	if isLocalFromTag || isLocalToTag {
		uutImages := os.Getenv(uutImagesEnvVar)
		if uutImages == "" {
			panic(uutImagesEnvVar + " environment variable is required when FROM_TAG or TO_TAG is set to \"" + localTagName + "\"")
		}

		localProviderPackage, localControllerPackage := GetImagesFromJsonOrPanic(uutImages)
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

		if pullPackages {
			mustPullImage(fromProviderPackage)
			mustPullImage(fromControllerPackage)
		}
	}

	if !isLocalToTag {
		toProviderPackage = fmt.Sprintf("%s:%s", toProviderRepository, toTag)
		toControllerPackage = fmt.Sprintf("%s:%s", toControllerRepository, toTag)

		if pullPackages {
			mustPullImage(toProviderPackage)
			mustPullImage(toControllerPackage)
		}
	}

	return fromProviderPackage, toProviderPackage, fromControllerPackage, toControllerPackage
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

func DeploymentRuntimeConfig(namePrefix string) vendored.DeploymentRuntimeConfig {
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

func LoadDurationMins(envVar string, defaultValue int) time.Duration {
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

func GetEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return fallback
}
