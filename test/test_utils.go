package test

import (
	"context"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/logging"
	"github.com/crossplane-contrib/xp-testing/pkg/xpenvfuncs"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metaApi "github.com/sap/crossplane-provider-btp/apis"
	apiV1Alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

func SetupLogging(verbosity int) {
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
