//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpmeta "github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"

	meta "github.com/sap/crossplane-provider-btp/apis"

	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	sacCreateName = "sac-subaccountapicredentials"
)

func TestSubaccountApiCredentialsStandalone(t *testing.T) {
	var manifestDir = crsPath("SubaccountApiCredentialsStandalone")
	var baselineSecretData map[string][]byte
	var baselineSecretType corev1.SecretType
	var baselineSecretOwnerReferences []metav1.OwnerReference
	reconcileNonce := 0

	crudFeature := features.New("SubaccountApiCredentials Creation Flow").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				resources.ImportResources(ctx, t, cfg, manifestDir)
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta.AddToScheme(r.GetScheme())

				sac := v1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{Name: sacCreateName, Namespace: cfg.Namespace()},
				}
				waitForResource(&sac, cfg, t, wait.WithTimeout(time.Minute*7))
				return ctx
			},
		).
		Assess(
			"Await resources to become synced",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &v1alpha1.SubaccountApiCredential{}
				MustGetResource(t, cfg, sacCreateName, nil, sac)
				waitForApiCredentialSynced(t, ctx, cfg, sacCreateName, corev1.ConditionTrue, "")

				secret := getApiCredentialSecret(t, ctx, cfg, sac)
				baselineSecretData = copySecretData(secret.Data)
				baselineSecretType = secret.Type
				baselineSecretOwnerReferences = append([]metav1.OwnerReference(nil), secret.OwnerReferences...)
				assertApiCredentialSecret(t, ctx, cfg, sac)

				return ctx
			},
		).
		Assess(
			"Issue #863: malformed connection Secrets are unhealthy and recover",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				if len(baselineSecretData) == 0 {
					t.Fatal("baseline connection Secret was not captured")
				}

				sac := &v1alpha1.SubaccountApiCredential{}
				MustGetResource(t, cfg, sacCreateName, nil, sac)
				secretRef := sac.GetWriteConnectionSecretToReference()
				if secretRef == nil {
					t.Fatal("SubaccountApiCredential has no connection Secret reference")
				}

				cases := []struct {
					name             string
					deleteSecret     bool
					data             func() map[string][]byte
					expectedKeys     []string
					messageSubstring string
				}{
					{
						name:             "missing destination Secret",
						deleteSecret:     true,
						messageSubstring: "connection secret",
					},
					{
						name: "empty Secret",
						data: func() map[string][]byte {
							return map[string][]byte{}
						},
						expectedKeys:     []string{},
						messageSubstring: "attribute.api_url",
					},
					{
						name: "client-ID-only Secret",
						data: func() map[string][]byte {
							return copySecretFields(baselineSecretData, "attribute.client_id")
						},
						expectedKeys:     []string{"attribute.client_id"},
						messageSubstring: "attribute.api_url",
					},
					{
						name: "client-ID-and-secret-only Secret",
						data: func() map[string][]byte {
							return copySecretFields(baselineSecretData, "attribute.client_id", "attribute.client_secret")
						},
						expectedKeys:     []string{"attribute.client_id", "attribute.client_secret"},
						messageSubstring: "attribute.api_url",
					},
					{
						name: "unrelated-data-only Secret",
						data: func() map[string][]byte {
							return map[string][]byte{"issue863.unrelated": []byte("fixture")}
						},
						expectedKeys:     []string{"issue863.unrelated"},
						messageSubstring: "attribute.api_url",
					},
					{
						name: "missing-client-secret Secret",
						data: func() map[string][]byte {
							return copySecretFields(baselineSecretData, "attribute.api_url", "attribute.client_id", "attribute.token_url")
						},
						expectedKeys:     []string{"attribute.api_url", "attribute.client_id", "attribute.token_url"},
						messageSubstring: "attribute.client_secret",
					},
				}

				for _, tc := range cases {
					tc := tc
					if ok := t.Run(tc.name, func(t *testing.T) {
						if tc.deleteSecret {
							secret := getApiCredentialSecret(t, ctx, cfg, sac)
							AwaitResourceDeletionOrFail(ctx, t, cfg, secret, wait.WithTimeout(time.Minute*2))
						} else {
							secret := getApiCredentialSecret(t, ctx, cfg, sac)
							secret.Data = tc.data()
							if err := cfg.Client().Resources().Update(ctx, secret); err != nil {
								t.Fatalf("failed to apply malformed connection Secret: %v", err)
							}
						}
						requestApiCredentialReconciliation(t, ctx, cfg, sacCreateName, &reconcileNonce)

						waitForApiCredentialSynced(t, ctx, cfg, sacCreateName, corev1.ConditionFalse, tc.messageSubstring)
						if tc.expectedKeys != nil {
							assertApiCredentialSecretKeys(t, ctx, cfg, secretRef, tc.expectedKeys)
						}

						secret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:            secretRef.Name,
								Namespace:       secretRef.Namespace,
								OwnerReferences: append([]metav1.OwnerReference(nil), baselineSecretOwnerReferences...),
							},
							Type: baselineSecretType,
							Data: copySecretData(baselineSecretData),
						}
						if tc.deleteSecret {
							if err := cfg.Client().Resources().Create(ctx, secret); err != nil {
								t.Fatalf("failed to restore connection Secret: %v", err)
							}
						} else {
							current := getApiCredentialSecret(t, ctx, cfg, sac)
							current.Data = secret.Data
							if err := cfg.Client().Resources().Update(ctx, current); err != nil {
								t.Fatalf("failed to restore connection Secret: %v", err)
							}
						}
						requestApiCredentialReconciliation(t, ctx, cfg, sacCreateName, &reconcileNonce)
						waitForApiCredentialSynced(t, ctx, cfg, sacCreateName, corev1.ConditionTrue, "")
						assertApiCredentialSecret(t, ctx, cfg, sac)
					}); !ok {
						// A failed subtest may leave an invalid Secret behind. The feature
						// teardown can still delete the managed resource, but do not run
						// more mutations against an unknown state.
						return ctx
					}
				}
				return ctx
			},
		).
		Assess(
			"Check Resources Delete",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				// k8s resource cleaned up?
				sac := &v1alpha1.SubaccountApiCredential{}
				MustGetResource(t, cfg, sacCreateName, nil, sac)

				AwaitResourceDeletionOrFail(ctx, t, cfg, sac, wait.WithTimeout(time.Minute*5))
				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			DeleteResourcesIgnoreMissing(ctx, t, cfg, manifestDir, wait.WithTimeout(time.Minute*5))
			return ctx
		},
	).Feature()

	testenv.Test(t, crudFeature)
}

func assertApiCredentialSecret(t *testing.T, ctx context.Context, cfg *envconf.Config, sac *v1alpha1.SubaccountApiCredential) {
	t.Helper()
	secret := getApiCredentialSecret(t, ctx, cfg, sac)
	for _, key := range []string{
		"attribute.api_url",
		"attribute.client_id",
		"attribute.client_secret",
		"attribute.token_url",
	} {
		if len(secret.Data[key]) == 0 {
			t.Errorf("connection Secret is missing a non-empty %s field", key)
		}
	}
}

func getApiCredentialSecret(t *testing.T, ctx context.Context, cfg *envconf.Config, sac *v1alpha1.SubaccountApiCredential) *corev1.Secret {
	t.Helper()
	secretRef := sac.GetWriteConnectionSecretToReference()
	if secretRef == nil {
		t.Fatal("SubaccountApiCredential has no connection Secret reference")
	}
	secret := &corev1.Secret{}
	if err := cfg.Client().Resources().Get(ctx, secretRef.Name, secretRef.Namespace, secret); err != nil {
		t.Fatalf("failed to load connection Secret: %v", err)
	}
	return secret
}

func copySecretData(data map[string][]byte) map[string][]byte {
	copied := make(map[string][]byte, len(data))
	for key, value := range data {
		copied[key] = append([]byte(nil), value...)
	}
	return copied
}

func copySecretFields(data map[string][]byte, fields ...string) map[string][]byte {
	result := make(map[string][]byte, len(fields))
	for _, field := range fields {
		result[field] = append([]byte(nil), data[field]...)
	}
	return result
}

func assertApiCredentialSecretKeys(t *testing.T, ctx context.Context, cfg *envconf.Config, ref *xpv1.SecretReference, expected []string) {
	t.Helper()
	secret := &corev1.Secret{}
	if err := cfg.Client().Resources().Get(ctx, ref.Name, ref.Namespace, secret); err != nil {
		t.Fatalf("failed to load malformed connection Secret: %v", err)
	}
	if len(secret.Data) != len(expected) {
		t.Fatalf("connection Secret has %d keys, expected %d", len(secret.Data), len(expected))
	}
	for _, key := range expected {
		if _, ok := secret.Data[key]; !ok {
			t.Errorf("connection Secret is missing expected key %s", key)
		}
	}
}

// Secret updates are not required to enqueue a managed resource. Explicitly
// changing a harmless annotation makes each assertion exercise the next
// reconciliation rather than depending on the controller's poll/backoff.
func requestApiCredentialReconciliation(t *testing.T, ctx context.Context, cfg *envconf.Config, name string, nonce *int) {
	t.Helper()
	credential := &v1alpha1.SubaccountApiCredential{}
	if err := cfg.Client().Resources().Get(ctx, name, "", credential); err != nil {
		t.Fatalf("failed to load SubaccountApiCredential for reconciliation request: %v", err)
	}
	annotations := credential.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	*nonce = *nonce + 1
	annotations["issue863.e2e/reconcile"] = fmt.Sprintf("%d", *nonce)
	credential.SetAnnotations(annotations)
	if err := cfg.Client().Resources().Update(ctx, credential); err != nil {
		t.Fatalf("failed to request SubaccountApiCredential reconciliation: %v", err)
	}
}

func waitForApiCredentialSynced(t *testing.T, ctx context.Context, cfg *envconf.Config, name string, expected corev1.ConditionStatus, messageSubstring string) {
	t.Helper()
	object := &v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: name}}
	match := conditions.New(cfg.Client().Resources()).ResourceMatch(object, func(object k8s.Object) bool {
		credential, ok := object.(*v1alpha1.SubaccountApiCredential)
		if !ok {
			return false
		}
		condition := credential.GetCondition(xpv1.TypeSynced)
		if condition.Status != expected {
			return false
		}
		return messageSubstring == "" || strings.Contains(strings.ToLower(condition.Message), strings.ToLower(messageSubstring))
	})
	if err := wait.For(match, wait.WithTimeout(time.Minute*7)); err != nil {
		t.Fatalf("timed out waiting for Synced=%s: %v", expected, err)
	}
}

// TestSubaccountApiCredentialExternalNameADRCompliance verifies that a newly
// provisioned SubaccountApiCredential gets an ADR-compliant external-name
// annotation in the compound `<subaccount-id>/<name>` format and produces a
// connection secret containing a valid client_secret.
//
// Note: the BTP Terraform provider does not implement ImportState for this resource type,
// so this test does not validate adoption/import of an existing credential.
func TestSubaccountApiCredentialExternalNameADRCompliance(t *testing.T) {
	var orphanManifestDir = crsPath("SubaccountApiCredentialOrphanImport")
	// Prefix "sac-adr-" ensures the BTP credential name starts with a letter.
	// The SAC is created in Setup (not Assess) so the controller picks it up
	// reliably while Crossplane is already actively reconciling resources.
	sacName := "sac-adr-" + BUILD_ID

	orphanImportFeature := features.New("SubaccountApiCredential External Name ADR Compliance").
		Setup(
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				r, _ := res.New(cfg.Client().RESTConfig())
				_ = meta.AddToScheme(r.GetScheme())

				// Apply the subaccount first. The SAC is created programmatically after
				// the subaccount is Ready to avoid exponential back-off on SubaccountRef
				// resolution.
				resources.ImportResources(ctx, t, cfg, orphanManifestDir)

				waitForResource(&accountv1alpha1.Subaccount{
					ObjectMeta: metav1.ObjectMeta{Name: "sac-orphan-subaccount", Namespace: cfg.Namespace()},
				}, cfg, t, wait.WithTimeout(time.Minute*12))

				// Create the SAC in Setup so the controller picks it up while Crossplane
				// is already actively reconciling. Do not set the external-name annotation
				// manually here: this is a creation test, not an adoption/import test. The
				// controller will default/reconstruct the compound external-name.
				readOnly := false
				sac := &v1alpha1.SubaccountApiCredential{
					ObjectMeta: metav1.ObjectMeta{Name: sacName, Namespace: cfg.Namespace()},
					Spec: v1alpha1.SubaccountApiCredentialSpec{
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{
								Name:      sacName + "-secret",
								Namespace: cfg.Namespace(),
							},
						},
						ForProvider: v1alpha1.SubaccountApiCredentialParameters{
							ReadOnly:      &readOnly,
							SubaccountRef: &xpv1.Reference{Name: "sac-orphan-subaccount"},
						},
					},
				}
				if err := cfg.Client().Resources().Create(ctx, sac); err != nil {
					t.Fatalf("Failed to create SAC: %v", err)
				}
				waitForResource(sac, cfg, t, wait.WithTimeout(time.Minute*8))

				return ctx
			},
		).
		Assess(
			"SAC external-name annotation uses compound key after provisioning (ADR compliance)",
			func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
				sac := &v1alpha1.SubaccountApiCredential{}
				MustGetResource(t, cfg, sacName, nil, sac)

				// ADR compliance: after provisioning, GetExternalNameFn reads `subaccount_id`
				// and `name` from Terraform state and writes the compound key back to the annotation.
				externalName := xpmeta.GetExternalName(sac)
				parts := strings.SplitN(externalName, "/", 2)
				if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
					t.Errorf("External name ADR compliance: annotation %q is not in <subaccount-id>/<name> format", externalName)
				}

				// Verify the connection secret contains a valid client_secret.
				assertApiCredentialSecret(t, ctx, cfg, sac)

				return ctx
			},
		).Teardown(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			sac := &v1alpha1.SubaccountApiCredential{}
			if err := cfg.Client().Resources().Get(ctx, sacName, cfg.Namespace(), sac); err == nil {
				AwaitResourceDeletionOrFail(ctx, t, cfg, sac, wait.WithTimeout(time.Minute*5))
			}
			DeleteResourcesIgnoreMissing(ctx, t, cfg, orphanManifestDir, wait.WithTimeout(time.Minute*5))
			return ctx
		},
	).Feature()

	testenv.Test(t, orphanImportFeature)
}
