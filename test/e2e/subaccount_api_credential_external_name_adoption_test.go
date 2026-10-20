//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	"github.com/crossplane-contrib/xp-testing/pkg/xpenvfuncs"
	xpmeta "github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const unsupportedCredentialAdoption = "adoption of an existing SubaccountApiCredential is not supported"

// TestSubaccountApiCredentialExternalNameRejectsAdoption verifies the annotated
// orphan-and-redeploy lifecycle across provider restarts. Publication of public
// connection fields is permitted; successful acquisition/readiness is not.
func TestSubaccountApiCredentialExternalNameRejectsAdoption(t *testing.T) {
	testCredentialExternalNameAdoption(t, true)
}

// TestSubaccountApiCredentialExternalNameRejectsAdoptionWithoutRestart runs the
// same lifecycle without restarting the provider between adoption and cleanup.
func TestSubaccountApiCredentialExternalNameRejectsAdoptionWithoutRestart(t *testing.T) {
	testCredentialExternalNameAdoption(t, false)
}

func testCredentialExternalNameAdoption(t *testing.T, restartProvider bool) {
	t.Helper()
	suffix := "restart"
	if !restartProvider {
		suffix = "no-restart"
	}
	base := "sac-868-" + suffix + "-" + BUILD_ID
	subaccountName := base + "-subaccount"
	credentialName := base + "-credential"
	firstName, secondName, probeName := base+"-first", base+"-adopt", base+"-probe"
	firstSecret, secondSecret, probeSecret := firstName+"-secret", secondName+"-secret", probeName+"-secret"
	var externalName string

	feature := features.New("SubaccountApiCredential annotated adoption rejection "+suffix).
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Register cleanup before creating any external resources. Cleanup uses
			// normal MR deletion and does not bypass finalizers or usage protection.
			t.Cleanup(func() {
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 24*time.Minute)
				defer cancel()
				kube := cfg.Client().Resources().GetControllerRuntimeClient()
				for _, name := range []string{firstName, secondName, probeName} {
					if err := cleanupCredentialMR(cleanupCtx, kube, name, 5*time.Minute); err != nil {
						t.Errorf("cleanup incomplete for credential MR %q (external identity %q): %v; confirm BTP cleanup before rerunning", name, externalName, err)
					}
				}
				if err := cleanupSubaccountMR(cleanupCtx, kube, subaccountName, 12*time.Minute); err != nil {
					t.Errorf("cleanup incomplete for subaccount MR %q (credential %q, identity %q): %v; cluster destruction does not remove BTP resources, confirm cleanup before rerunning", subaccountName, credentialName, externalName, err)
				}
			})

			subaccount := &accountv1alpha1.Subaccount{
				ObjectMeta: metav1.ObjectMeta{Name: subaccountName},
				Spec: accountv1alpha1.SubaccountSpec{ForProvider: accountv1alpha1.SubaccountParameters{
					DisplayName:      base + "-subaccount",
					Region:           "eu10",
					Subdomain:        base + "-subdomain",
					Labels:           map[string]accountv1alpha1.SubaccountLabelValueList{"safe-to-delete": {"yes"}, "BUILD_ID": {BUILD_ID}},
					SubaccountAdmins: []string{envvar.GetOrPanic(TECHNICAL_USER_EMAIL_ENV_KEY)},
				}},
			}
			if err := cfg.Client().Resources().Create(ctx, subaccount); err != nil {
				t.Fatalf("create scenario-specific subaccount: %v", err)
			}
			waitForResource(subaccount, cfg, t, wait.WithTimeout(12*time.Minute))

			first := newAdoptionTestCredential(firstName, firstSecret, cfg.Namespace(), credentialName, subaccountName, xpv1.DeletionOrphan)
			if err := cfg.Client().Resources().Create(ctx, first); err != nil {
				t.Fatalf("create initial credential: %v", err)
			}
			waitForResource(first, cfg, t, wait.WithTimeout(8*time.Minute))
			created := &v1alpha1.SubaccountApiCredential{}
			MustGetResource(t, cfg, firstName, nil, created)
			assertAdoptionTestSecret(t, ctx, cfg, created, firstSecret)
			externalName = xpmeta.GetExternalName(created)
			if created.Spec.ForProvider.SubaccountID == nil || externalName != *created.Spec.ForProvider.SubaccountID+"/"+credentialName {
				t.Fatalf("created external-name %q does not match requested credential identity", externalName)
			}
			return ctx
		}).Assess("reject adoption, finalize locally, and verify BTP credential survives", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		first := &v1alpha1.SubaccountApiCredential{}
		MustGetResource(t, cfg, firstName, nil, first)
		AwaitResourceDeletionOrFail(ctx, t, cfg, first, wait.WithTimeout(5*time.Minute))
		if restartProvider {
			restartCredentialTestProvider(t, ctx, cfg)
		}

		if err := cfg.Client().Resources().Get(ctx, secondSecret, cfg.Namespace(), &corev1.Secret{}); !apierrors.IsNotFound(err) {
			t.Fatalf("fresh adoption Secret should not exist before adoption attempt; Get() error = %v", err)
		}
		second := newAdoptionTestCredential(secondName, secondSecret, cfg.Namespace(), credentialName, subaccountName, xpv1.DeletionDelete)
		xpmeta.SetExternalName(second, externalName)
		if err := cfg.Client().Resources().Create(ctx, second); err != nil {
			t.Fatalf("create annotated adoption MR: %v", err)
		}

		if err := waitForAdoptionFailure(ctx, cfg, secondName, 8*time.Minute); err != nil {
			logCredentialDiagnostics(t, ctx, cfg, secondName)
			t.Errorf("expected explicit unsupported-adoption diagnostic: %v", err)
		}
		adopted := &v1alpha1.SubaccountApiCredential{}
		if err := cfg.Client().Resources().Get(ctx, secondName, "", adopted); err != nil {
			t.Fatalf("get annotated adoption MR: %v", err)
		}
		if ready := adopted.GetCondition(xpv1.TypeReady); ready.Status == corev1.ConditionTrue {
			t.Errorf("unsupported adoption must not become Ready=True; condition=%#v", ready)
		}
		if synced := adopted.GetCondition(xpv1.TypeSynced); synced.Status != corev1.ConditionFalse || !strings.Contains(strings.ToLower(synced.Message), strings.ToLower(unsupportedCredentialAdoption)) {
			t.Errorf("expected Synced=False with explicit adoption diagnostic, got %#v", synced)
		}
		// Connection-detail publication can be partial under the revised
		// contract; it is not used as evidence of successful acquisition.

		if restartProvider {
			restartCredentialTestProvider(t, ctx, cfg)
		}
		AwaitResourceDeletionOrFail(ctx, t, cfg, adopted, wait.WithTimeout(5*time.Minute))

		// A new MR performs a real provider Observe against the compound
		// identity. Its explicit rejection proves the original target still
		// exists; arbitrary reconciliation errors are not accepted as proof.
		probe := newAdoptionTestCredential(probeName, probeSecret, cfg.Namespace(), credentialName, subaccountName, xpv1.DeletionOrphan)
		xpmeta.SetExternalName(probe, externalName)
		if err := cfg.Client().Resources().Create(ctx, probe); err != nil {
			t.Fatalf("create independent BTP existence probe: %v", err)
		}
		if err := waitForAdoptionFailure(ctx, cfg, probeName, 8*time.Minute); err != nil {
			logCredentialDiagnostics(t, ctx, cfg, probeName)
			t.Fatalf("probe did not independently confirm that the original credential still exists: %v", err)
		}
		return ctx
	}).Feature()

	testenv.Test(t, feature)
}

func newAdoptionTestCredential(resourceName, secretName, namespace, credentialName, subaccountName string, policy xpv1.DeletionPolicy) *v1alpha1.SubaccountApiCredential {
	readOnly := false
	return &v1alpha1.SubaccountApiCredential{
		ObjectMeta: metav1.ObjectMeta{Name: resourceName},
		Spec: v1alpha1.SubaccountApiCredentialSpec{
			ClusterManagedResourceSpec: xpv1.ClusterManagedResourceSpec{
				DeletionPolicy:                   policy,
				WriteConnectionSecretToReference: &xpv1.SecretReference{Name: secretName, Namespace: namespace},
			},
			ForProvider: v1alpha1.SubaccountApiCredentialParameters{
				Name: &credentialName, ReadOnly: &readOnly, SubaccountRef: &xpv1.Reference{Name: subaccountName},
			},
		},
	}
}

func waitForAdoptionFailure(ctx context.Context, cfg *envconf.Config, name string, timeout time.Duration) error {
	resource := &v1alpha1.SubaccountApiCredential{}
	return wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(resource, func(object k8s.Object) bool {
		got := object.(*v1alpha1.SubaccountApiCredential)
		synced := got.GetCondition(xpv1.TypeSynced)
		return synced.Status == corev1.ConditionFalse && synced.ObservedGeneration >= got.Generation && strings.Contains(strings.ToLower(synced.Message), strings.ToLower(unsupportedCredentialAdoption))
	}), wait.WithTimeout(timeout), wait.WithContext(ctx))
}

func assertAdoptionTestSecret(t *testing.T, ctx context.Context, cfg *envconf.Config, cr *v1alpha1.SubaccountApiCredential, name string) {
	t.Helper()
	secret := &corev1.Secret{}
	ref := cr.GetWriteConnectionSecretToReference()
	if ref == nil {
		t.Fatal("initial credential has no connection Secret destination")
	}
	if err := cfg.Client().Resources().Get(ctx, name, ref.Namespace, secret); err != nil {
		t.Fatalf("get initial credential Secret: %v", err)
	}
	if len(secret.Data["attribute.client_secret"]) == 0 {
		t.Fatal("initial provider-created Secret has no client_secret")
	}
}

func logCredentialDiagnostics(t *testing.T, ctx context.Context, cfg *envconf.Config, name string) {
	t.Helper()
	cr := &v1alpha1.SubaccountApiCredential{}
	if err := cfg.Client().Resources().Get(ctx, name, "", cr); err != nil {
		t.Logf("get credential MR %q for diagnostics: %v", name, err)
		return
	}
	ready, synced := cr.GetCondition(xpv1.TypeReady), cr.GetCondition(xpv1.TypeSynced)
	t.Logf("credential MR %q generation=%d Ready=%s/%s Synced=%s/%s observedGeneration=%d message=%q finalizers=%v", name, cr.Generation, ready.Status, ready.Reason, synced.Status, synced.Reason, synced.ObservedGeneration, synced.Message, cr.Finalizers)
}

func cleanupCredentialMR(ctx context.Context, kube client.Client, name string, timeout time.Duration) error {
	cr := &v1alpha1.SubaccountApiCredential{}
	if err := kube.Get(ctx, client.ObjectKey{Name: name}, cr); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("get MR: %w", err)
	}
	if err := kube.Delete(ctx, cr); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("request normal MR deletion: %w", err)
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := kube.Get(ctx, client.ObjectKey{Name: name}, cr); apierrors.IsNotFound(err) {
			return nil
		} else if err != nil {
			return fmt.Errorf("check MR finalization: %w", err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("cleanup context ended while MR remained: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("timed out after %s with MR still present and finalizers %v", timeout, cr.Finalizers)
		case <-ticker.C:
		}
	}
}

func cleanupSubaccountMR(ctx context.Context, kube client.Client, name string, timeout time.Duration) error {
	cr := &accountv1alpha1.Subaccount{}
	if err := kube.Get(ctx, client.ObjectKey{Name: name}, cr); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("get MR: %w", err)
	}
	if err := kube.Delete(ctx, cr); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("request normal MR deletion: %w", err)
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := kube.Get(ctx, client.ObjectKey{Name: name}, cr); apierrors.IsNotFound(err) {
			return nil
		} else if err != nil {
			return fmt.Errorf("check MR finalization: %w", err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("cleanup context ended while MR remained: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("timed out after %s with MR still present and finalizers %v", timeout, cr.Finalizers)
		case <-ticker.C:
		}
	}
}

func restartCredentialTestProvider(t *testing.T, ctx context.Context, cfg *envconf.Config) {
	t.Helper()
	kube := cfg.Client().Resources().GetControllerRuntimeClient()
	pods := &corev1.PodList{}
	if err := kube.List(ctx, pods, client.InNamespace(xpenvfuncs.CrossplaneNamespace)); err != nil {
		t.Fatalf("list provider pods: %v", err)
	}
	var providerPods []corev1.Pod
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			if strings.Contains(container.Image, "/provider-btp-") {
				providerPods = append(providerPods, pod)
				break
			}
		}
	}
	if len(providerPods) != 1 {
		t.Fatalf("expected exactly one BTP provider pod, got %d", len(providerPods))
	}
	old := providerPods[0]
	if err := kube.Delete(ctx, &old, client.Preconditions{UID: &old.UID}); err != nil {
		t.Fatalf("delete provider pod %q: %v", old.Name, err)
	}
	if err := wait.For(func(ctx context.Context) (bool, error) {
		current := &corev1.PodList{}
		if err := kube.List(ctx, current, client.InNamespace(old.Namespace)); err != nil {
			return false, nil
		}
		for _, pod := range current.Items {
			if pod.UID == old.UID || pod.DeletionTimestamp != nil {
				continue
			}
			for _, container := range pod.Spec.Containers {
				if strings.Contains(container.Image, "/provider-btp-") {
					for _, condition := range pod.Status.Conditions {
						if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
							return true, nil
						}
					}
				}
			}
		}
		return false, nil
	}, wait.WithTimeout(5*time.Minute), wait.WithContext(ctx)); err != nil {
		t.Fatalf("wait for replacement BTP provider pod: %v", err)
	}
}
