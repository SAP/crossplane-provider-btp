//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/resources"
	xpmeta "github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const (
	credentialAdoptionError = "cannot adopt existing SubaccountApiCredential: Terraform state does not provide a complete connection Secret"
)

var credentialConnectionSecretKeys = []string{
	"attribute.client_id",
	"attribute.client_secret",
	"attribute.api_url",
	"attribute.token_url",
}

var credentialDiagnosticSensitiveValue = regexp.MustCompile(`(?i)(client[_-]?(?:secret|id)|password|authorization|access[_-]?token|refresh[_-]?token|token|api[_-]?url|token[_-]?url)(["']?\s*[:=]\s*["']?)[^"',\s}\]]+`)

// TestSubaccountApiCredentialConditionalAdoption exercises adoption against a
// credential created in this disposable E2E environment. The second MRs use
// Observe-only to avoid competing writes/deletes; the provider does not enforce
// that policy as an adoption requirement.
func TestSubaccountApiCredentialConditionalAdoption(t *testing.T) {
	const manifestDir = "SubaccountApiCredentialOrphanImport"
	const timeout = 8 * time.Minute
	feature := features.New("SubaccountApiCredential Conditional Adoption").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			resources.ImportResources(ctx, t, cfg, crsPath(manifestDir))
			waitForResource(&accountv1alpha1.Subaccount{
				ObjectMeta: metav1.ObjectMeta{Name: "sac-orphan-subaccount", Namespace: cfg.Namespace()},
			}, cfg, t, wait.WithTimeout(12*time.Minute))

			created := createCredentialForAdoptionE2E(ctx, t, cfg, "sac-adopt-source-"+BUILD_ID, "sac-adopt-source-secret-"+BUILD_ID, nil)
			waitForResource(created, cfg, t, wait.WithTimeout(timeout))
			created = getCredentialForAdoptionE2E(ctx, t, cfg, created.Name)
			assertCompleteCredentialSecretE2E(t, ctx, cfg, created)
			assertCredentialIdentityE2E(t, created)

			// The Framework reconstructs sensitive observation from the configured
			// destination Secret. Seed only the secret in a distinct destination;
			// Read must obtain the public fields from the existing target.
			completeSecret := getCredentialSecretE2E(ctx, t, cfg, created)
			completeAdopterName := "sac-adopt-complete-" + BUILD_ID
			// Seed only the write-once secret; successful observation must supply
			// public fields and publish the complete result into this partial target.
			createCredentialSecretE2E(ctx, t, cfg, completeAdopterName+"-secret", map[string][]byte{
				"attribute.client_secret": append([]byte(nil), completeSecret.Data["attribute.client_secret"]...),
			})
			completeAdopter := createCredentialForAdoptionE2E(
				ctx, t, cfg, completeAdopterName, completeAdopterName+"-secret", created,
			)
			t.Logf("source before adoption: %s", credentialAdoptionDiagnosticsE2E(ctx, cfg, created.Name))
			t.Logf("adopter before observation: %s", credentialAdoptionDiagnosticsE2E(ctx, cfg, completeAdopter.Name))
			if !waitForCredentialReadyAndSyncedE2E(ctx, t, cfg, completeAdopter.Name, timeout) {
				t.Logf("source at adoption timeout (before teardown): %s", credentialAdoptionDiagnosticsE2E(ctx, cfg, created.Name))
				t.Logf("subaccount at adoption timeout (before teardown): %s", credentialAdoptionSubaccountDiagnosticsE2E(ctx, cfg))
				return ctx
			}
			completeAdopter = getCredentialForAdoptionE2E(ctx, t, cfg, completeAdopter.Name)
			assertCompleteCredentialSecretE2E(t, ctx, cfg, completeAdopter)
			assertCredentialIdentityE2E(t, completeAdopter)
			adoptedSecret := getCredentialSecretE2E(ctx, t, cfg, completeAdopter)
			if !bytes.Equal(adoptedSecret.Data["attribute.client_secret"], completeSecret.Data["attribute.client_secret"]) {
				t.Error("adoption changed the established client_secret")
			}

			// An existing but incomplete destination lets the Framework observe the
			// BTP target while proving that public fields alone are not adoption.
			incompleteAdopterName := "sac-adopt-incomplete-" + BUILD_ID
			createCredentialSecretE2E(ctx, t, cfg, incompleteAdopterName+"-secret", map[string][]byte{})
			incompleteAdopter := createCredentialForAdoptionE2E(
				ctx, t, cfg, incompleteAdopterName, incompleteAdopterName+"-secret", created,
			)
			waitForIncompleteCredentialAdoptionE2E(ctx, t, cfg, incompleteAdopter.Name, timeout)

			return ctx
		}).Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		// Remove the Observe-only MRs before deleting the source credential.
		for _, name := range []string{
			"sac-adopt-incomplete-" + BUILD_ID,
			"sac-adopt-complete-" + BUILD_ID,
		} {
			mr := &securityv1alpha1.SubaccountApiCredential{}
			if err := cfg.Client().Resources().Get(ctx, name, cfg.Namespace(), mr); err == nil {
				AwaitResourceDeletionOrFail(ctx, t, cfg, mr, wait.WithTimeout(3*time.Minute))
			}
		}
		source := &securityv1alpha1.SubaccountApiCredential{}
		if err := cfg.Client().Resources().Get(ctx, "sac-adopt-source-"+BUILD_ID, cfg.Namespace(), source); err == nil {
			AwaitResourceDeletionOrFail(ctx, t, cfg, source, wait.WithTimeout(5*time.Minute))
		}
		for _, name := range []string{
			"sac-adopt-source-secret-" + BUILD_ID,
			"sac-adopt-complete-" + BUILD_ID + "-secret",
			"sac-adopt-incomplete-" + BUILD_ID + "-secret",
		} {
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cfg.Namespace()}}
			if err := cfg.Client().Resources().Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
				t.Errorf("delete test connection Secret %q: %v", name, err)
			}
		}
		DeleteResourcesIgnoreMissing(ctx, t, cfg, crsPath(manifestDir), wait.WithTimeout(5*time.Minute))
		return ctx
	}).Feature()

	testenv.Test(t, feature)
}

func createCredentialForAdoptionE2E(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
	name string,
	secretName string,
	established *securityv1alpha1.SubaccountApiCredential,
) *securityv1alpha1.SubaccountApiCredential {
	t.Helper()
	readOnly := false
	policy := []xpv1.ManagementAction{xpv1.ManagementActionObserve}
	credential := &securityv1alpha1.SubaccountApiCredential{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cfg.Namespace()},
		Spec: securityv1alpha1.SubaccountApiCredentialSpec{
			ClusterManagedResourceSpec: xpv1.ClusterManagedResourceSpec{
				ManagementPolicies: policy,
				WriteConnectionSecretToReference: &xpv1.SecretReference{
					Name: secretName, Namespace: cfg.Namespace(),
				},
			},
			ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
				Name:     &name,
				ReadOnly: &readOnly,
			},
		},
	}
	if established == nil {
		// The original resource is created normally; permit normal create/update
		// behavior and explicitly retain Delete for its teardown.
		credential.Spec.ManagementPolicies = []xpv1.ManagementAction{
			xpv1.ManagementActionObserve,
			xpv1.ManagementActionCreate,
			xpv1.ManagementActionUpdate,
			xpv1.ManagementActionDelete,
			xpv1.ManagementActionLateInitialize,
		}
		credential.Spec.ForProvider.SubaccountRef = &xpv1.Reference{Name: "sac-orphan-subaccount"}
		if err := cfg.Client().Resources().Create(ctx, credential); err != nil {
			t.Fatalf("create source SubaccountApiCredential %q: %v", name, err)
		}
		return credential
	}

	subaccountID := established.Status.AtProvider.SubaccountID
	if subaccountID == nil || *subaccountID == "" {
		t.Fatal("created credential has no observed subaccount ID")
	}
	credentialName := valueForAdoptionE2E(established.Status.AtProvider.Name)
	if credentialName == "" {
		t.Fatal("created credential has no observed name")
	}
	credential.Spec.ForProvider.Name = &credentialName
	credential.Spec.ForProvider.SubaccountID = subaccountID
	credential.Spec.ManagementPolicies = policy
	credential.Annotations = map[string]string{
		xpmeta.AnnotationKeyExternalName: fmt.Sprintf("%s/%s", *subaccountID, credentialName),
	}
	if err := cfg.Client().Resources().Create(ctx, credential); err != nil {
		t.Fatalf("create adoption MR %q: %v", name, err)
	}
	return credential
}

func valueForAdoptionE2E(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func getCredentialForAdoptionE2E(ctx context.Context, t *testing.T, cfg *envconf.Config, name string) *securityv1alpha1.SubaccountApiCredential {
	t.Helper()
	credential := &securityv1alpha1.SubaccountApiCredential{}
	if err := cfg.Client().Resources().Get(ctx, name, cfg.Namespace(), credential); err != nil {
		t.Fatalf("get SubaccountApiCredential %q: %v", name, err)
	}
	return credential
}

func getCredentialSecretE2E(ctx context.Context, t *testing.T, cfg *envconf.Config, credential *securityv1alpha1.SubaccountApiCredential) *corev1.Secret {
	t.Helper()
	ref := credential.GetWriteConnectionSecretToReference()
	secret := &corev1.Secret{}
	if err := cfg.Client().Resources().Get(ctx, ref.Name, ref.Namespace, secret); err != nil {
		t.Fatalf("get connection Secret for %q: %v", credential.Name, err)
	}
	return secret
}

func createCredentialSecretE2E(ctx context.Context, t *testing.T, cfg *envconf.Config, name string, data map[string][]byte) {
	t.Helper()
	copied := make(map[string][]byte, len(data))
	for key, value := range data {
		copied[key] = append([]byte(nil), value...)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cfg.Namespace()},
		Type:       xpresource.SecretTypeConnection,
		Data:       copied,
	}
	if err := cfg.Client().Resources().Create(ctx, secret); err != nil {
		t.Fatalf("create destination Secret %q: %v", name, err)
	}
}

func waitForCredentialReadyAndSyncedE2E(ctx context.Context, t *testing.T, cfg *envconf.Config, name string, timeout time.Duration) bool {
	t.Helper()
	match := conditions.New(cfg.Client().Resources()).ResourceMatch(
		&securityv1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cfg.Namespace()}},
		func(object k8s.Object) bool {
			credential, ok := object.(*securityv1alpha1.SubaccountApiCredential)
			return ok &&
				credential.GetCondition(xpv1.TypeReady).Status == corev1.ConditionTrue &&
				credential.GetCondition(xpv1.TypeSynced).Status == corev1.ConditionTrue
		},
	)
	err := wait.For(match, wait.WithTimeout(timeout))
	if err == nil {
		return true
	}
	details := credentialAdoptionDiagnosticsE2E(ctx, cfg, name)
	t.Errorf("credential %q did not become Ready and Synced within %s: %v; diagnostics: %s", name, timeout, err, details)
	return false
}

func credentialAdoptionDiagnosticsE2E(ctx context.Context, cfg *envconf.Config, name string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	credential := &securityv1alpha1.SubaccountApiCredential{}
	if err := cfg.Client().Resources().Get(ctx, name, cfg.Namespace(), credential); err != nil {
		return fmt.Sprintf("could not read MR: %v", err)
	}
	conditionsSummary := make([]string, 0, 2)
	// Available is a Ready condition, not a separate condition type.
	for _, conditionType := range []xpv1.ConditionType{xpv1.TypeSynced, xpv1.TypeReady} {
		condition := credential.GetCondition(conditionType)
		message := credentialDiagnosticSensitiveValue.ReplaceAllString(condition.Message, "$1$2[REDACTED]")
		if len(message) > 512 {
			message = message[:512] + "…"
		}
		reason := credentialDiagnosticSensitiveValue.ReplaceAllString(string(condition.Reason), "$1$2[REDACTED]")
		conditionsSummary = append(conditionsSummary, fmt.Sprintf("%s=%s/%s message=%q", condition.Type, condition.Status, reason, message))
	}
	observedName := valueForAdoptionE2E(credential.Status.AtProvider.Name)
	observedSubaccountID := valueForAdoptionE2E(credential.Status.AtProvider.SubaccountID)
	secretSummary := "connection Secret not configured"
	if ref := credential.GetWriteConnectionSecretToReference(); ref != nil {
		secret := &corev1.Secret{}
		if err := cfg.Client().Resources().Get(ctx, ref.Name, ref.Namespace, secret); err != nil {
			state := "read failed"
			if errors.IsNotFound(err) {
				state = "not found"
			} else if errors.IsForbidden(err) {
				state = "forbidden"
			}
			secretSummary = fmt.Sprintf("connection Secret %s/%s %s", ref.Namespace, ref.Name, state)
		} else {
			present := make([]string, 0, len(credentialConnectionSecretKeys))
			for _, key := range credentialConnectionSecretKeys {
				present = append(present, fmt.Sprintf("%s=%t", key, len(secret.Data[key]) != 0))
			}
			secretSummary = fmt.Sprintf("connection Secret %s/%s key-presence: %s", ref.Namespace, ref.Name, strings.Join(present, ","))
		}
	}
	return fmt.Sprintf(
		"generation=%d deleting=%t policies=%v external-name=%q spec-subaccount=%q spec-name=%q observed-subaccount=%q observed-name=%q conditions=[%s] %s",
		credential.Generation,
		!credential.DeletionTimestamp.IsZero(),
		credential.Spec.ManagementPolicies,
		xpmeta.GetExternalName(credential),
		valueForAdoptionE2E(credential.Spec.ForProvider.SubaccountID),
		valueForAdoptionE2E(credential.Spec.ForProvider.Name),
		observedSubaccountID,
		observedName,
		strings.Join(conditionsSummary, "; "),
		secretSummary,
	)
}

// This snapshot is Kubernetes evidence only, not an independent BTP presence
// check. Collect it before teardown so later absence is not misattributed to Read.
func credentialAdoptionSubaccountDiagnosticsE2E(ctx context.Context, cfg *envconf.Config) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	subaccount := &accountv1alpha1.Subaccount{}
	if err := cfg.Client().Resources().Get(ctx, "sac-orphan-subaccount", cfg.Namespace(), subaccount); err != nil {
		return fmt.Sprintf("could not read subaccount MR (%T)", err)
	}
	ready := subaccount.GetCondition(xpv1.TypeReady)
	synced := subaccount.GetCondition(xpv1.TypeSynced)
	return fmt.Sprintf("external-name=%q deleting=%t Ready=%s/%s Synced=%s/%s", xpmeta.GetExternalName(subaccount), !subaccount.DeletionTimestamp.IsZero(), ready.Status, ready.Reason, synced.Status, synced.Reason)
}

func assertCredentialIdentityE2E(t *testing.T, credential *securityv1alpha1.SubaccountApiCredential) {
	t.Helper()
	subaccountID := credential.Status.AtProvider.SubaccountID
	name := credential.Status.AtProvider.Name
	if subaccountID == nil || *subaccountID == "" || name == nil || *name == "" {
		t.Errorf("credential %q has incomplete observed identity", credential.Name)
		return
	}
	expected := fmt.Sprintf("%s/%s", *subaccountID, *name)
	if actual := xpmeta.GetExternalName(credential); actual != expected {
		t.Errorf("credential %q external name = %q, want %q", credential.Name, actual, expected)
	}
}

func assertCompleteCredentialSecretE2E(t *testing.T, ctx context.Context, cfg *envconf.Config, credential *securityv1alpha1.SubaccountApiCredential) {
	t.Helper()
	secret := getCredentialSecretE2E(ctx, t, cfg, credential)
	for _, key := range credentialConnectionSecretKeys {
		if len(secret.Data[key]) == 0 {
			t.Errorf("connection Secret %q lacks nonempty required key %q", secret.Name, key)
		}
	}
}

func waitForIncompleteCredentialAdoptionE2E(ctx context.Context, t *testing.T, cfg *envconf.Config, name string, timeout time.Duration) {
	t.Helper()
	// Poll the live object because this negative outcome intentionally cannot use
	// waitForResource, which waits exclusively for Available=True.
	match := conditions.New(cfg.Client().Resources()).ResourceMatch(
		&securityv1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cfg.Namespace()}},
		func(object k8s.Object) bool {
			credential, ok := object.(*securityv1alpha1.SubaccountApiCredential)
			if !ok {
				return false
			}
			synced := credential.GetCondition(xpv1.TypeSynced)
			ready := credential.GetCondition(xpv1.TypeReady)
			return synced.Status == corev1.ConditionFalse &&
				ready.Status == corev1.ConditionFalse &&
				strings.Contains(synced.Message, credentialAdoptionError)
		},
	)
	if err := wait.For(match, wait.WithTimeout(timeout)); err != nil {
		t.Fatalf("incomplete adoption did not reach explicit rejection: %v; diagnostics: %s", err, credentialAdoptionDiagnosticsE2E(ctx, cfg, name))
	}
}
