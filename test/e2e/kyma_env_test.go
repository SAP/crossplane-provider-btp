//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"encoding/json"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"k8s.io/apimachinery/pkg/runtime"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"

	meta "github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"

	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestKymaEnvironment(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping kyma in short mode")
		return
	}
	var manifestDir = "testdata/crs/kyma_env"
	crudFeature := features.New("BTP Kyma Environment Controller").
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
				if err := resources.WaitForResourcesToBeSynced(ctx, cfg, manifestDir, nil, wait.WithTimeout(time.Minute*50)); err != nil {
					t.Fatal(err)
				}
				return ctx
			},
		).
		Assess(
			"Check Resources Delete",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.DeleteResources(ctx, t, cfg, manifestDir, wait.WithTimeout(time.Minute*50))
				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			DeleteResourcesIgnoreMissing(ctx, t, cfg, manifestDir, wait.WithTimeout(time.Minute*5))
			return ctx
		},
	).
		Teardown(resources.DumpManagedResources).
		Feature()

	testenv.Test(t, crudFeature)
}

func TestKymaEnvironmentImportFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping kyma import in short mode")
		return
	}

	kymaImportName := "e2e-kyma-import-test"

	// Prepare parameters as RawExtension
	parameters := map[string]interface{}{
		"region":         "westeurope",
		"administrators": []string{envvar.GetOrPanic(TECHNICAL_USER_EMAIL_ENV_KEY)},
	}
	parametersJSON, err := json.Marshal(parameters)
	if err != nil {
		t.Fatalf("failed to marshal parameters: %v", err)
	}

	importTester := NewImportTester(
		&v1alpha1.KymaEnvironment{
			Spec: v1alpha1.KymaEnvironmentSpec{
				ForProvider: v1alpha1.KymaEnvironmentParameters{
					PlanName:   "azure",
					Name:       &kymaImportName,
					Parameters: runtime.RawExtension{Raw: parametersJSON},
				},
				SubaccountRef: &xpv1.Reference{
					Name: "kyma-import-test-subaccount",
				},
				CloudManagementRef: &xpv1.Reference{
					Name: "cis-local-kyma-import",
				},
			},
		},
		kymaImportName,
		WithWaitDependentResourceTimeout[*v1alpha1.KymaEnvironment](wait.WithTimeout(15*time.Minute)),
		WithWaitCreateTimeout[*v1alpha1.KymaEnvironment](wait.WithTimeout(50*time.Minute)),
		WithWaitDeletionTimeout[*v1alpha1.KymaEnvironment](wait.WithTimeout(50*time.Minute)),
		WithDependentResourceDirectory[*v1alpha1.KymaEnvironment]("testdata/crs/kyma_env_import"),
	)

	importFeature := importTester.BuildTestFeature("BTP Kyma Environment Import Flow").Feature()

	testenv.Test(t, importFeature)
}
