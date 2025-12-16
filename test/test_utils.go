package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/logging"
	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	"github.com/crossplane-contrib/xp-testing/pkg/xpenvfuncs"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metaApi "github.com/sap/crossplane-provider-btp/apis"
	apiV1Alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
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

func CreateProviderConfigFn(namespace string, globalAccount string, cliServerUrl string, cisSecretName string, serviceUserSecretName string) func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		r, _ := res.New(cfg.Client().RESTConfig())
		_ = metaApi.AddToScheme(r.GetScheme())

		obj := ProviderConfig(namespace, globalAccount, cliServerUrl, cisSecretName, serviceUserSecretName)
		err := r.Create(ctx, obj)
		if kubeErrors.IsAlreadyExists(err) {
			return ctx, r.Update(ctx, obj)
		}
		return ctx, err
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
