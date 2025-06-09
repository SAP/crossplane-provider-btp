//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"

	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"

	meta "github.com/sap/crossplane-provider-btp/apis"

	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestSubaccountApiCredentialsIntegration(t *testing.T) {
	var manifestDir = "testdata/crs/SubaccountApiCredentialsIntegration"
	crudFeature := features.New("SubaccountApiCredentials Integration Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, manifestDir)
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta.AddToScheme(r.GetScheme())
				return ctx
			},
		).
		Assess(
			"Await resources to become synced",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				if err := resources.WaitForResourcesToBeSynced(ctx, cfg, manifestDir, wait.WithTimeout(time.Minute*7)); err != nil {
					t.Fatal(err)
				}
				return ctx
			},
		).
		Assess(
			"Check Resources Delete",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.DeleteResources(ctx, t, cfg, manifestDir, wait.WithTimeout(time.Minute*7))
				return ctx
			},
		).
		Teardown(resources.DumpManagedResources).
		Feature()

	testenv.Test(t, crudFeature)
}
