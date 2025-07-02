//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/wait"

	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

var (
	sbCreateName = "e2e-destination-binding"
)

func TestServiceBinding_CreationFlow(t *testing.T) {
	crudFeatureSuite := features.New("ServiceBinding Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, "testdata/crs/servicebinding")
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
			DeleteResourcesIgnoreMissing(ctx, t, cfg, "serviceinstance", wait.WithTimeout(time.Minute*5))
			return ctx
		},
	).Feature()

	testenv.Test(t, crudFeatureSuite)
}
