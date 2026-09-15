//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

// TestSubaccountDestinationCertificate_CreationFlow tests create, update (content change),
// and delete of a SubaccountDestinationCertificate resource.
func TestSubaccountDestinationCertificate_CreationFlow(t *testing.T) {
	certCreateName := "e2e-cert-" + BUILD_ID
	crudFeatureSuite := features.New("SubaccountDestinationCertificate Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, crsPath("SubaccountDestinationCertificate"))
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())

				// Wait for ServiceBinding — creates the destination credentials secret.
				sb := v1alpha1.ServiceBinding{
					ObjectMeta: metav1.ObjectMeta{Name: "e2e-dest-cert-binding", Namespace: cfg.Namespace()},
				}
				waitForResource(&sb, cfg, t, wait.WithTimeout(15*time.Minute))

				// Wait for the SubaccountDestinationCertificate to become Available.
				cert := v1alpha1.SubaccountDestinationCertificate{
					ObjectMeta: metav1.ObjectMeta{Name: certCreateName, Namespace: cfg.Namespace()},
				}
				waitForResource(&cert, cfg, t, wait.WithTimeout(10*time.Minute))
				return ctx
			},
		).
		Assess(
			"Check SubaccountDestinationCertificate is fully created", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				cert := &v1alpha1.SubaccountDestinationCertificate{}
				MustGetResource(t, cfg, certCreateName, nil, cert)

				if cert.Status.AtProvider.Name == nil || *cert.Status.AtProvider.Name == "" {
					t.Error("SubaccountDestinationCertificate atProvider.name not set after creation")
				}

				return ctx
			},
		).
		Assess(
			"Drift detection: updating contentSecretRef triggers reconciliation and resource stays Available",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				cert := &v1alpha1.SubaccountDestinationCertificate{}
				MustGetResource(t, cfg, certCreateName, nil, cert)

				prevHash := ""
				if cert.Status.AtProvider.ContentHash != nil {
					prevHash = *cert.Status.AtProvider.ContentHash
				}

				// Switch to the second certificate key in the same secret.
				// The controller re-reads the secret on every Connect, so this
				// causes isUpToDate to return false and triggers an Update to BTP.
				updated := cert.DeepCopy()
				updated.Spec.ForProvider.ContentSecretRef = &xpv1.SecretKeySelector{
					SecretReference: xpv1.SecretReference{
						Name:      cert.Spec.ForProvider.ContentSecretRef.Name,
						Namespace: cert.Spec.ForProvider.ContentSecretRef.Namespace,
					},
					Key: "content-v2",
				}

				if err := cfg.Client().Resources().Update(ctx, updated); err != nil {
					t.Fatalf("failed to update contentSecretRef key: %v", err)
				}

				// Wait for atProvider.contentHash to change — the controller only
				// writes the new hash after a successful PUT to BTP, so a changed
				// hash proves the updated content reached the backend.
				resources.AwaitResourceUpdateFor(
					ctx, t, cfg, updated,
					func(obj k8s.Object) bool {
						c, ok := obj.(*v1alpha1.SubaccountDestinationCertificate)
						return ok && c.Status.AtProvider.ContentHash != nil &&
							*c.Status.AtProvider.ContentHash != prevHash
					},
					wait.WithTimeout(5*time.Minute),
				)
				return ctx
			},
		).
		Assess(
			"Properly delete all resources", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				cert := &v1alpha1.SubaccountDestinationCertificate{}
				MustGetResource(t, cfg, certCreateName, nil, cert)
				AwaitResourceDeletionOrFail(ctx, t, cfg, cert, wait.WithTimeout(3*time.Minute))
				return ctx
			},
		).
		Teardown(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				DeleteResourcesIgnoreMissing(ctx, t, cfg, crsPath("SubaccountDestinationCertificate"), wait.WithTimeout(5*time.Minute))
				return ctx
			},
		).Feature()

	testenv.Test(t, crudFeatureSuite)
}
