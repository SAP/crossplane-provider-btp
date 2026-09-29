//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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

				// ImportResources overrides metadata.namespace to cfg.Namespace() on every
				// imported object. The cert CR's contentSecretRef.namespace is a spec field —
				// it is NOT mutated — so it still points at crossplane-system. Re-create the
				// cert content secret explicitly in crossplane-system so Connect() can find it.
				certContentSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "e2e-cert-content",
						Namespace: "crossplane-system",
					},
					Type: corev1.SecretTypeOpaque,
					StringData: map[string]string{
						"content":    "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJlekNDQVNHZ0F3SUJBZ0lVRWdoOWEvanptSnhGanBwek1ScCt1UTkrckI0d0NnWUlLb1pJemowRUF3SXcKRXpFUk1BOEdBMVVFQXd3SVpUSmxMWFJsYzNRd0hoY05Nall3T1RBNU1UTXlOVEl3V2hjTk16WXdPVEEyTVRNeQpOVEl3V2pBVE1SRXdEd1lEVlFRRERBaGxNbVV0ZEdWemREQlpNQk1HQnlxR1NNNDlBZ0VHQ0NxR1NNNDlBd0VICkEwSUFCQnJHblppYlhnNGhKZVQ1RHpPc0hBcVp6b2QxUmZIc2Z1SXc2SmE3akdVcTFDbVdBcHcyZWdVQ2t4RU0KdWxYZ3grUnNsYk1RWXdCdC8ra3IxZURFRDAralV6QlJNQjBHQTFVZERnUVdCQlFzTEVoYXZKeUFNdnUwNEYzYgo5ZnBVaXRoY3lEQWZCZ05WSFNNRUdEQVdnQlFzTEVoYXZKeUFNdnUwNEYzYjlmcFVpdGhjeURBUEJnTlZIUk1CCkFmOEVCVEFEQVFIL01Bb0dDQ3FHU000OUJBTUNBMGdBTUVVQ0lDN3hpZkZPRUxpTDFvb1U4VkFGZ0NjeW5wZTAKMmt0a05rUGxIOHJTMzlVSUFpRUEweDJ5R2h5WldXRUtUR2tDZEdmVzBqd1VFV3RlaE16Z0hvK0h3aEpUK1h3PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==",
						"content-v2": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJnRENDQVNlZ0F3SUJBZ0lVTzJ5WkFvbmM0S01vWGxZc1QwdmFaNzI0U0Q0d0NnWUlLb1pJemowRUF3SXcKRmpFVU1CSUdBMVVFQXd3TFpUSmxMWFJsYzNRdGRqSXdIaGNOTWpZd09URTBNVE0wT1RRNFdoY05Nell3T1RFeApNVE0wT1RRNFdqQVdNUlF3RWdZRFZRUUREQXRsTW1VdGRHVnpkQzEyTWpCWk1CTUdCeXFHU000OUFnRUdDQ3FHClNNNDlBd0VIQTBJQUJOdlZKNzk0bitpa1lHQ2dQdHppM3gyTzJ6K1ZJaGFuSGk2UFNLSW00RE12UVlpR2lyQ2sKMzBHZnNOWElKN0dvQ0tMVlRCOTZxUXVoREZMeUpxbEpiOEdqVXpCUk1CMEdBMVVkRGdRV0JCVEFFM041aWJpRQpQZjBmaVNzZXcyTjJ3WjZvaFRBZkJnTlZIU01FR0RBV2dCVEFFM041aWJpRVBmMGZpU3NldzJOMndaNm9oVEFQCkJnTlZIUk1CQWY4RUJUQURBUUgvTUFvR0NDcUdTTTQ5QkFNQ0EwY0FNRVFDSUNuMDJCcDBaTktEOEg5N0QyZTMKYVJHcTUwL1N0d1RvSWhnUEl1TW5MZ09pQWlBSTRKWis0YW5sNVZWMjdXNlczQmNNZ25EcEZxeUxXUS9VUkxnKwpPWVRWTWc9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==",
					},
				}
				if err := cfg.Client().Resources().Create(ctx, certContentSecret); err != nil && !k8serrors.IsAlreadyExists(err) {
					t.Fatalf("failed to create cert content secret in crossplane-system: %v", err)
				}

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

				// Re-fetch to get the latest resourceVersion — the reconciler may
				// have patched the cert between the prevHash read and this update,
				// which would cause a 409 conflict on the update below.
				MustGetResource(t, cfg, certCreateName, nil, cert)

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
				certContentSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "e2e-cert-content", Namespace: "crossplane-system"},
				}
				if err := cfg.Client().Resources().Delete(ctx, certContentSecret); err != nil && !k8serrors.IsNotFound(err) {
					t.Logf("warning: failed to delete cert content secret from crossplane-system: %v", err)
				}
				return ctx
			},
		).Feature()

	testenv.Test(t, crudFeatureSuite)
}
