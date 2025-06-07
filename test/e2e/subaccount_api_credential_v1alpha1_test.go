//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"

	meta "github.com/sap/crossplane-provider-btp/apis"

	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	sacCreateName = "sac-subaccountapicredentials"
)

func TestSubaccountApiCredentialsStandalone(t *testing.T) {
	var manifestDir = "testdata/crs/SubaccountApiCredentialsStandalone"
	crudFeature := features.New("SubaccountApiCredentials Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, manifestDir)
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta.AddToScheme(r.GetScheme())

				sac := v1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{Name: sacCreateName, Namespace: cfg.Namespace()},
				}
				waitForResource(&sac, cfg, t, wait.WithTimeout(time.Minute*7))
				return ctx
			},
		).
		Assess(
			"Await resources to become synced",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &v1alpha1.SubaccountApiCredential{}
				MustGetResource(t, cfg, sacCreateName, nil, sac)

				assertApiCredentialSecret(t, ctx, cfg, sac)

				return ctx
			},
		).
		Assess(
			"Check Resources Delete",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				// k8s resource cleaned up?
				sac := &v1alpha1.SubaccountApiCredential{}
				MustGetResource(t, cfg, sacCreateName, nil, sac)

				AwaitResourceDeletionOrFail(ctx, t, cfg, sac, wait.WithTimeout(time.Minute*5))
				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			DeleteResourcesIgnoreMissing(ctx, t, cfg, manifestDir, wait.WithTimeout(time.Minute*5))
			return ctx
		},
	).Feature()

	testenv.Test(t, crudFeature)
}

func assertApiCredentialSecret(t *testing.T, ctx context.Context, cfg *envconf.Config, sac *v1alpha1.SubaccountApiCredential) {
	secretName := sac.GetWriteConnectionSecretToReference().Name
	secretNS := sac.GetWriteConnectionSecretToReference().Namespace
	secret := &corev1.Secret{}
	err := cfg.Client().Resources().Get(ctx, secretName, secretNS, secret)
	if err != nil {
		t.Error("Error while loading expected secret from Ref")
	}
	// secret contains correct structure
	if _, ok := secret.Data["attribute.client_secret"]; !ok {
		t.Error("Secret not in proper format")
	}
}
