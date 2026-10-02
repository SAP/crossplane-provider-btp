//go:build e2e_long

package e2e

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	var manifestDir = crsPath("kyma_env")
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
			// Regression guard for issue #682: after a BTP-side (Cockpit)
			// update, BTP materialises schema defaults such as
			// accessControlList:{}, gvisor:{enabled:false}, ingressFiltering:false
			// into the stored parameters. The schema-aware drift detector must
			// treat those as non-drift, so the provider neither loops on updates
			// nor trips the circuit breaker. We force one update round-trip (which
			// makes BTP echo the defaults back), then watch the RetryStatus over a
			// sustained window and assert the breaker stays off and the retry
			// counter does not climb.
			"Verify materialised defaults do not trip the circuit breaker (#682)",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				const kymaName = "kyma-environment"

				// Force an update round-trip against BTP so it writes the schema
				// defaults into status.atProvider.parameters, reproducing the
				// no-op Cockpit update from the bug report. An annotation bump
				// re-triggers reconciliation without changing the desired spec.
				kyma := &v1alpha1.KymaEnvironment{}
				MustGetResource(t, cfg, kymaName, nil, kyma)
				metav1.SetMetaDataAnnotation(&kyma.ObjectMeta, "e2e.btp.sap.crossplane.io/drift-probe", strconv.FormatInt(time.Now().Unix(), 10))
				if err := cfg.Client().Resources().Update(ctx, kyma); err != nil {
					t.Fatalf("failed to annotate KymaEnvironment to trigger reconcile: %v", err)
				}

				// Watch for a sustained window. If materialised defaults were
				// mistaken for drift, the provider would keep updating and
				// RetryStatus.Count would climb toward the circuit breaker; a
				// correct fix keeps Count low and CircuitBreaker false throughout.
				const (
					watchWindow  = 6 * time.Minute
					pollInterval = 15 * time.Second
					maxCount     = 2 // tolerate a single reconcile blip; well below the default maxRetries (3)
				)
				deadline := time.Now().Add(watchWindow)
				for time.Now().Before(deadline) {
					observed := &v1alpha1.KymaEnvironment{}
					MustGetResource(t, cfg, kymaName, nil, observed)

					if rs := observed.Status.RetryStatus; rs != nil {
						if rs.CircuitBreaker {
							t.Fatalf("circuit breaker tripped on materialised schema defaults (#682 regression); retryStatus=%+v", *rs)
						}
						if rs.Count > maxCount {
							t.Fatalf("retry count climbed to %d (>%d) on materialised schema defaults (#682 regression); diff=%q", rs.Count, maxCount, rs.Diff)
						}
					}

					// The resource must also stay healthy throughout.
					if cond := observed.GetCondition(xpv1.Available().Type); cond.Status != corev1.ConditionTrue {
						t.Logf("KymaEnvironment not (yet) Available during drift watch: %v", cond)
					}
					time.Sleep(pollInterval)
				}

				final := &v1alpha1.KymaEnvironment{}
				MustGetResource(t, cfg, kymaName, nil, final)
				if final.GetCondition(xpv1.Available().Type).Status != corev1.ConditionTrue {
					t.Fatalf("KymaEnvironment is not Available after the drift watch window; conditions=%+v", final.Status.Conditions)
				}
				t.Logf("no false drift over %s: retryStatus=%+v", watchWindow, final.Status.RetryStatus)
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
		WithDependentResourceDirectory[*v1alpha1.KymaEnvironment](crsPath("kyma_env_import")),
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
		WithDependentResourceDirectory[*v1alpha1.KymaModule](crsPath("kyma_module_import")),
	)

	importFeature := importTester.BuildTestFeature("BTP Kyma Module Import Flow").Feature()

	testenv.Test(t, importFeature)
}
