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
	siCreateName = "e2e-destination-instance"
)

func TestServiceInstance_CreationFlow(t *testing.T) {
	crudFeatureSuite := features.New("ServiceInstance Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, "testdata/crs/serviceinstance")
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())

				sm := v1alpha1.ServiceInstance{
					ObjectMeta: metav1.ObjectMeta{Name: siCreateName, Namespace: cfg.Namespace()},
				}
				waitForResource(&sm, cfg, t, wait.WithTimeout(7*time.Minute))
				return ctx
			},
		).
		Assess(
			"Check ServiceInstance Resources are fully created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				si := &v1alpha1.ServiceInstance{}
				MustGetResource(t, cfg, siCreateName, nil, si)
				// Status bound?
				if si.Status.AtProvider.ID == "" {
					t.Error("Serviceinstance not fully initialized")
				}
				return ctx
			},
		).Assess(
		"Properly delete all resources", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// k8s resource cleaned up?
			si := &v1alpha1.ServiceInstance{}
			MustGetResource(t, cfg, siCreateName, nil, si)

			AwaitResourceDeletionOrFail(ctx, t, cfg, si, wait.WithTimeout(time.Minute*5))

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
