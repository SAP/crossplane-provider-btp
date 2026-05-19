//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"

	meta "github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"

	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const (
	kymaModuleImportBindingRefName = "kyma-module-import-binding"
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

	importTester := NewImportTester(
		&v1alpha1.KymaEnvironment{
			Spec: v1alpha1.KymaEnvironmentSpec{
				ForProvider: v1alpha1.KymaEnvironmentParameters{
					PlanName: "azure",
					Name:     &kymaImportName,
					Parameters: runtime.RawExtension{
						Object: &unstructured.Unstructured{
							Object: map[string]any{
								"region":         "westeurope",
								"administrators": []any{envvar.GetOrPanic(TECHNICAL_USER_EMAIL_ENV_KEY)},
							},
						},
					},
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

// TestKymaModuleImportFlow tests the import flow for KymaModule resource
// according to the External Name Handling ADR.
//
// This test verifies that:
// 1. A KymaModule can be created with its dependencies (KymaEnvironment + KymaEnvironmentBinding)
// 2. The external-name is properly set to the module name (e.g. "cloud-manager")
// 3. The resource can be imported using the external-name annotation
// 4. Imported resources transition to healthy state
func TestKymaModuleImportFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping kyma module import in short mode")
		return
	}

	kymaModuleImportName := "cloud-manager"

	importTester := NewImportTester(
		&v1alpha1.KymaModule{
			Spec: v1alpha1.KymaModuleSpec{
				ForProvider: v1alpha1.KymaModuleParameters{
					Name: kymaModuleImportName,
				},
				KymaEnvironmentBindingRef: &xpv1.Reference{
					Name: kymaModuleImportBindingRefName,
				},
			},
		},
		kymaModuleImportName,
		WithWaitDependentResourceTimeout[*v1alpha1.KymaModule](wait.WithTimeout(60*time.Minute)),
		WithWaitCreateTimeout[*v1alpha1.KymaModule](wait.WithTimeout(15*time.Minute)),
		WithWaitDeletionTimeout[*v1alpha1.KymaModule](wait.WithTimeout(15*time.Minute)),
		WithDependentResourceDirectory[*v1alpha1.KymaModule]("testdata/crs/kyma_module_import"),
	)

	importFeature := importTester.BuildTestFeature("BTP Kyma Module Import Flow").Feature()

	testenv.Test(t, importFeature)
}
