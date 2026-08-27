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
	testutil "github.com/sap/crossplane-provider-btp/test"
)

var (
	destBindingSecret   = "e2e-destination-binding-secret"
	destBindingSecretNS = "crossplane-system"
	destServiceBinding  = "e2e-destination-binding"
)

// TestSubaccountDestination_CreationFlow tests create, update, and delete of a
// SubaccountDestination resource. It is fully self-contained: it provisions a
// fresh subaccount, entitlement, ServiceManager, ServiceInstance, and
// ServiceBinding for the Destination Service, then patches the ProviderConfig
// with destinationCredentials before creating the destination resource.
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
					ObjectMeta: metav1.ObjectMeta{Name: destServiceBinding, Namespace: cfg.Namespace()},
				}
				waitForResource(&sb, cfg, t, wait.WithTimeout(15*time.Minute))

				// Patch the ProviderConfig to add destinationCredentials pointing
				// at the connection secret written by the ServiceBinding.
				if err := testutil.PatchProviderConfigDestinationCredentials(
					ctx, cfg,
					destBindingSecret,
					destBindingSecretNS,
				); err != nil {
					t.Fatalf("failed to patch ProviderConfig with destinationCredentials: %v", err)
				}

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

				updatedURL := "https://updated.example.com"
				updated := dest.DeepCopy()
				updated.Spec.ForProvider.URL = &updatedURL
				resources.AwaitResourceUpdateOrError(ctx, t, cfg, updated)

				// Wait for the controller to reconcile the update to BTP and reflect
				// the new URL in atProvider.
				resources.AwaitResourceUpdateFor(
					ctx, t, cfg, updated,
					func(obj k8s.Object) bool {
						d, ok := obj.(*v1alpha1.SubaccountDestination)
						return ok && d.Status.AtProvider.URL != nil && *d.Status.AtProvider.URL == updatedURL
					},
					wait.WithTimeout(3*time.Minute),
				)

				after := &v1alpha1.SubaccountDestination{}
				MustGetResource(t, cfg, destCreateName, nil, after)
				if after.Status.AtProvider.URL == nil || *after.Status.AtProvider.URL != updatedURL {
					t.Errorf("atProvider.url = %v, want %q", after.Status.AtProvider.URL, updatedURL)
				}

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
				// Remove destinationCredentials from ProviderConfig to leave it clean.
				_ = testutil.PatchProviderConfigDestinationCredentials(ctx, cfg, "", "")
				DeleteResourcesIgnoreMissing(ctx, t, cfg, crsPath("SubaccountDestination"), wait.WithTimeout(5*time.Minute))
				return ctx
			},
		).Feature()

	testenv.Test(t, crudFeatureSuite)
}
