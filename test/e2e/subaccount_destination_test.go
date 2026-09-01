//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

// TestSubaccountDestination_CreationFlow tests create, update, and delete of a
// SubaccountDestination resource. It is fully self-contained: it provisions a
// fresh subaccount, entitlement, ServiceManager, ServiceInstance, and
// ServiceBinding for the Destination Service, then creates the destination
// resource once the binding secret is available.
func TestSubaccountDestination_CreationFlow(t *testing.T) {
	destCreateName := "e2e-dest-" + BUILD_ID
	crudFeatureSuite := features.New("SubaccountDestination Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, crsPath("SubaccountDestination"))
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())

				// Wait for ServiceBinding — slowest step, creates the destination credentials secret.
				sb := v1alpha1.ServiceBinding{
					ObjectMeta: metav1.ObjectMeta{Name: "e2e-destination-binding", Namespace: cfg.Namespace()},
				}
				waitForResource(&sb, cfg, t, wait.WithTimeout(15*time.Minute))

				// Now wait for the SubaccountDestination to become Available.
				dest := v1alpha1.SubaccountDestination{
					ObjectMeta: metav1.ObjectMeta{Name: destCreateName, Namespace: cfg.Namespace()},
				}
				waitForResource(&dest, cfg, t, wait.WithTimeout(10*time.Minute))
				return ctx
			},
		).
		Assess(
			"Check SubaccountDestination is fully created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				dest := &v1alpha1.SubaccountDestination{}
				MustGetResource(t, cfg, destCreateName, nil, dest)

				if dest.Status.AtProvider.Name == nil || *dest.Status.AtProvider.Name == "" {
					t.Error("SubaccountDestination atProvider.name not set after creation")
				}
				if dest.Status.AtProvider.ETag == nil || *dest.Status.AtProvider.ETag == "" {
					t.Error("SubaccountDestination atProvider.etag not set after creation")
				}

				return ctx
			},
		).
		Assess(
			"Check SubaccountDestination update is reflected", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				dest := &v1alpha1.SubaccountDestination{}
				MustGetResource(t, cfg, destCreateName, nil, dest)

				prevETag := ""
				if dest.Status.AtProvider.ETag != nil {
					prevETag = *dest.Status.AtProvider.ETag
				}

				updatedURL := "https://updated.example.com"
				updated := dest.DeepCopy()
				if updated.Spec.ForProvider.AdditionalProperties == nil {
					updated.Spec.ForProvider.AdditionalProperties = map[string]string{}
				}
				updated.Spec.ForProvider.AdditionalProperties["URL"] = updatedURL
				resources.AwaitResourceUpdateOrError(ctx, t, cfg, updated)

				// Wait for ETag to change — BTP issues a new ETag on every successful PUT,
				// so this confirms the update was actually accepted by BTP.
				resources.AwaitResourceUpdateFor(
					ctx, t, cfg, updated,
					func(obj k8s.Object) bool {
						d, ok := obj.(*v1alpha1.SubaccountDestination)
						return ok && d.Status.AtProvider.ETag != nil &&
							*d.Status.AtProvider.ETag != prevETag
					},
					wait.WithTimeout(3*time.Minute),
				)

				return ctx
			},
		).
		Assess(
			"Properly delete all resources", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				dest := &v1alpha1.SubaccountDestination{}
				MustGetResource(t, cfg, destCreateName, nil, dest)
				AwaitResourceDeletionOrFail(ctx, t, cfg, dest, wait.WithTimeout(3*time.Minute))
				return ctx
			},
		).
		Teardown(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				DeleteResourcesIgnoreMissing(ctx, t, cfg, crsPath("SubaccountDestination"), wait.WithTimeout(5*time.Minute))
				return ctx
			},
		).Feature()

	testenv.Test(t, crudFeatureSuite)
}
