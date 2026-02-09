//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	meta "github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"

	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestCloudFoundryEnvironment(t *testing.T) {
	var manifestDir = "testdata/crs/cloudfoundry_env"

	crudFeature := features.New("BTP CF Environment Controller").
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
				if err := resources.WaitForResourcesToBeSynced(ctx, cfg, manifestDir, nil, wait.WithTimeout(time.Minute*25)); err != nil {
					t.Fatal(err)
				}
				return ctx
			},
		).
		Assess(
			"Check Resources Delete",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.DeleteResources(ctx, t, cfg, manifestDir, wait.WithTimeout(time.Minute*25))
				return ctx
			},
		).
		Teardown(resources.DumpManagedResources).
		Feature()

	testenv.Test(t, crudFeature)
}

func TestCloudFoundryEnvironmentImportFlow(t *testing.T) {
	importTester := NewImportTester(
		&v1alpha1.CloudFoundryEnvironment{
			Spec: v1alpha1.CfEnvironmentSpec{
				ForProvider: v1alpha1.CfEnvironmentParameters{
					Landscape:       "cf-eu10",
					EnvironmentName: "e2e-cf-import-test",
				},
			},
		},
		"e2e-cf-import-test",
		WithWaitCreateTimeout[*v1alpha1.CloudFoundryEnvironment](wait.WithTimeout(10*time.Minute)),
		WithWaitDeletionTimeout[*v1alpha1.CloudFoundryEnvironment](wait.WithTimeout(10*time.Minute)),
		WithDependentResourceDirectory[*v1alpha1.CloudFoundryEnvironment]("testdata/crs/cloudfoundry_env"),
	)

	importFeature := importTester.BuildTestFeature("BTP CF Environment Import Flow").Feature()

	testenv.Test(t, importFeature)
}
