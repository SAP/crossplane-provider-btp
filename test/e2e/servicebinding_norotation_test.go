//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
)

var (
	sbCreateName = "e2e-destination-binding"
)

func TestServiceBinding_CreationFlow(t *testing.T) {
	crudFeatureSuite := features.New("ServiceBinding Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, crsPath("servicebinding/env"))
				resources.ImportResources(ctx, t, cfg, crsPath("servicebinding/no-rotation"))
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())

				sb := v1alpha1.ServiceBinding{
					ObjectMeta: metav1.ObjectMeta{Name: sbCreateName, Namespace: cfg.Namespace()},
				}
				waitForResource(&sb, cfg, t, wait.WithTimeout(7*time.Minute))
				return ctx
			},
		).
		Assess(
			"Check ServiceBinding Resources are fully created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sb := &v1alpha1.ServiceBinding{}
				MustGetResource(t, cfg, sbCreateName, nil, sb)
				// Status bound?
				if sb.Status.AtProvider.ID == "" {
					t.Error("ServiceBinding not fully initialized")
				}
				if sb.Status.AtProvider.Name != sb.Spec.ForProvider.Name {
					t.Error("ServiceBinding status name is not the name as the spec says. Maybe it got generated like if rotation is enabled? (It is not)")
				}
				return ctx
			},
		).Assess(
		"Check ServiceBinding secret carries flattened keys plus __raw blob", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			sb := &v1alpha1.ServiceBinding{}
			MustGetResource(t, cfg, sbCreateName, nil, sb)
			secretRef := sb.GetWriteConnectionSecretToReference()
			secret := &corev1.Secret{}
			MustGetResource(t, cfg, secretRef.Name, &secretRef.Namespace, secret)
			rawBlob, ok := secret.Data[providerv1alpha1.RawBindingKey]
			if !ok || len(rawBlob) == 0 {
				t.Errorf("ServiceBinding secret missing %q key with raw credentials blob", providerv1alpha1.RawBindingKey)
				return ctx
			}
			var rawCreds map[string]any
			if err := json.Unmarshal(rawBlob, &rawCreds); err != nil {
				t.Errorf("ServiceBinding secret %q value is not valid JSON: %v", providerv1alpha1.RawBindingKey, err)
				return ctx
			}
			if len(rawCreds) == 0 {
				t.Errorf("ServiceBinding secret %q blob has no fields", providerv1alpha1.RawBindingKey)
			}
			return ctx
		},
	).Assess(
		"Properly delete all resources", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// k8s resource cleaned up?
			sb := &v1alpha1.ServiceBinding{}
			MustGetResource(t, cfg, sbCreateName, nil, sb)

			AwaitResourceDeletionOrFail(ctx, t, cfg, sb, wait.WithTimeout(time.Minute*5))

			return ctx
		},
	).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			DeleteResourcesIgnoreMissing(ctx, t, cfg, "servicebinding/env", wait.WithTimeout(time.Minute*5))
			return ctx
		},
	).Feature()

	testenv.Test(t, crudFeatureSuite)
}
