//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

var (
	destCreateName = "e2e-dest-" + BUILD_ID
)

// TestSubaccountDestination_CreationFlow tests create, update, and delete of a
// SubaccountDestination resource against a real BTP subaccount.
//
// Prerequisites:
//   - ProviderConfig named "e2e-provider-config" with destinationCredentials
//     referencing a Kubernetes Secret containing Destination Service binding
//     credentials (clientid, clientsecret, tokenurl, uri).
//   - The secret must be pre-created in the cluster before running this test.
func TestSubaccountDestination_CreationFlow(t *testing.T) {
	crudFeatureSuite := features.New("SubaccountDestination Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, crsPath("SubaccountDestination"))
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())

				dest := v1alpha1.SubaccountDestination{
					ObjectMeta: metav1.ObjectMeta{Name: destCreateName, Namespace: cfg.Namespace()},
				}
				waitForResource(&dest, cfg, t, wait.WithTimeout(5*time.Minute))
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

				// Verify atProvider reflects the update
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
				DeleteResourcesIgnoreMissing(ctx, t, cfg, "SubaccountDestination", wait.WithTimeout(5*time.Minute))
				return ctx
			},
		).Feature()

	testenv.Test(t, crudFeatureSuite)
}
