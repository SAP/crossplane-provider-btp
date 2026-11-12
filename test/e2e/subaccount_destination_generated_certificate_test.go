//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

const genCertPasswordSecretName = "e2e-gencert-password"

// TestSubaccountDestinationGeneratedCertificate_CreationFlow tests create,
// spec immutability and delete of SubaccountDestinationGeneratedCertificate
// resources in both .pem and .p12 format.
func TestSubaccountDestinationGeneratedCertificate_CreationFlow(t *testing.T) {
	pemName := "e2e-gencert-pem-" + BUILD_ID
	p12Name := "e2e-gencert-p12-" + BUILD_ID
	p12CommonName := "e2e-gencert-" + BUILD_ID
	certNames := []string{pemName, p12Name}

	crudFeatureSuite := features.New("SubaccountDestinationGeneratedCertificate Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, crsPath("SubaccountDestinationGeneratedCertificate"))
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = apis.AddToScheme(r.GetScheme())

				// The p12 CR's passwordSecretRef points at crossplane-system. Create the
				// password secret there explicitly: ImportResources would override the
				// namespace of a Secret imported from testdata.
				passwordSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      genCertPasswordSecretName,
						Namespace: "crossplane-system",
					},
					Type: corev1.SecretTypeOpaque,
					StringData: map[string]string{
						"password": "E2e-" + BUILD_ID + "-pw",
					},
				}
				if err := cfg.Client().Resources().Create(ctx, passwordSecret); err != nil && !k8serrors.IsAlreadyExists(err) {
					t.Fatalf("failed to create gencert password secret in crossplane-system: %v", err)
				}

				// Wait for ServiceBinding — creates the destination credentials secret.
				sb := v1alpha1.ServiceBinding{
					ObjectMeta: metav1.ObjectMeta{Name: "e2e-dest-gencert-binding", Namespace: cfg.Namespace()},
				}
				waitForResource(&sb, cfg, t, wait.WithTimeout(15*time.Minute))

				for _, name := range certNames {
					cert := v1alpha1.SubaccountDestinationGeneratedCertificate{
						ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cfg.Namespace()},
					}
					waitForResource(&cert, cfg, t, wait.WithTimeout(10*time.Minute))
				}
				return ctx
			},
		).
		Assess(
			"status reports the leaf certificate", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				for _, name := range certNames {
					cert := &v1alpha1.SubaccountDestinationGeneratedCertificate{}
					MustGetResource(t, cfg, name, nil, cert)

					if got := ptr.Deref(cert.Status.AtProvider.Name, ""); got != cert.Spec.ForProvider.Name {
						t.Errorf("%s: atProvider.name = %q, want %q", name, got, cert.Spec.ForProvider.Name)
					}
					c := cert.Status.AtProvider.Certificate
					if c == nil {
						t.Errorf("%s: atProvider.certificate not set", name)
						continue
					}
					if c.SerialNumber == "" {
						t.Errorf("%s: atProvider.certificate.serialNumber is empty", name)
					}
					if c.SHA256Fingerprint == "" {
						t.Errorf("%s: atProvider.certificate.sha256Fingerprint is empty", name)
					}
					if !c.NotAfter.After(time.Now()) {
						t.Errorf("%s: atProvider.certificate.notAfter %s is not in the future", name, c.NotAfter)
					}
					if name == p12Name && c.CommonName != p12CommonName {
						t.Errorf("%s: atProvider.certificate.commonName = %q, want %q", name, c.CommonName, p12CommonName)
					}
				}
				return ctx
			},
		).
		Assess(
			"spec is immutable", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				cert := &v1alpha1.SubaccountDestinationGeneratedCertificate{}
				MustGetResource(t, cfg, p12Name, nil, cert)

				// A merge patch carries no resourceVersion, so a concurrent status
				// write by the reconciler can't turn this into a 409 Conflict.
				patch := k8s.Patch{
					PatchType: types.MergePatchType,
					Data:      []byte(fmt.Sprintf(`{"spec":{"forProvider":{"commonName":%q}}}`, p12CommonName+"-changed")),
				}
				err := cfg.Client().Resources().Patch(ctx, cert, patch)
				if err == nil {
					t.Fatal("expected update of commonName to be rejected, got no error")
				}
				if !strings.Contains(err.Error(), "commonName can't be added, changed or removed") {
					t.Fatalf("unexpected error updating commonName: %v", err)
				}
				return ctx
			},
		).
		Assess(
			"Properly delete all resources", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				for _, name := range certNames {
					cert := &v1alpha1.SubaccountDestinationGeneratedCertificate{}
					MustGetResource(t, cfg, name, nil, cert)
					AwaitResourceDeletionOrFail(ctx, t, cfg, cert, wait.WithTimeout(3*time.Minute))
				}
				return ctx
			},
		).
		Teardown(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				DeleteResourcesIgnoreMissing(ctx, t, cfg, crsPath("SubaccountDestinationGeneratedCertificate"), wait.WithTimeout(5*time.Minute))
				passwordSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: genCertPasswordSecretName, Namespace: "crossplane-system"},
				}
				if err := cfg.Client().Resources().Delete(ctx, passwordSecret); err != nil && !k8serrors.IsNotFound(err) {
					t.Logf("warning: failed to delete gencert password secret from crossplane-system: %v", err)
				}
				return ctx
			},
		).Feature()

	testenv.Test(t, crudFeatureSuite)
}
