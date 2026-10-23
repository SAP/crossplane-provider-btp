//go:build e2e
// +build e2e

package e2e

import (
	"context"
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
	res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// TestSubaccountApiCredentialExternalNameRejectsAdoption tests full management,
// not Observe-only import. A new MR supplies an existing credential's compound
// external-name, with no previous status or connection Secret. The annotation
// must not authorize adoption or deletion, including after a provider restart.
func TestSubaccountApiCredentialExternalNameRejectsAdoption(t *testing.T) {
	testSubaccountApiCredentialExternalNameRejectsAdoption(t, true)
}

// TestSubaccountApiCredentialExternalNameRejectsAdoptionWithoutRestart isolates
// annotation-based adoption and cleanup while the original provider process
// remains running. Neither the adoption attempt nor deletion restarts it.
func TestSubaccountApiCredentialExternalNameRejectsAdoptionWithoutRestart(t *testing.T) {
	testSubaccountApiCredentialExternalNameRejectsAdoption(t, false)
}

func testSubaccountApiCredentialExternalNameRejectsAdoption(t *testing.T, restartProvider bool) {
	t.Helper()
	prefix := "sac-868-ann-"
	scenario := "with provider restart"
	subaccountScenario := "restart"
	if !restartProvider {
		prefix = "sac-868-ann-nr-"
		scenario = "without provider restart"
		subaccountScenario = "no-restart"
	}
	subaccountResourceName := "sac-orphan-subaccount-" + subaccountScenario
	credentialName := prefix + BUILD_ID
	firstName := prefix + "first-" + BUILD_ID
	secondName := prefix + "adopt-" + BUILD_ID
	probeName := prefix + "probe-" + BUILD_ID
	firstSecret := firstName + "-secret"
	secondSecret := secondName + "-secret"
	probeSecret := probeName + "-secret"
	var externalName string

	feature := features.New("SubaccountApiCredential supplied external-name rejects adoption "+scenario).
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Register before setup creates anything: Fatal/FailNow, including
			// setup failures, must not skip best-effort external cleanup.
			t.Cleanup(func() {
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 24*time.Minute)
				defer cancel()
				kube := cfg.Client().Resources().GetControllerRuntimeClient()
				credentials := []client.Object{
					&v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: firstName}},
					&v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: secondName}},
					&v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: probeName}},
				}
				credentialErr := deleteCredentialAdoptionTestResources(cleanupCtx, kube, credentials, t.Logf, wait.WithTimeout(5*time.Minute))
				// Always attempt the dedicated subaccount, even if an MR is stuck.
				// Normal deletion preserves the provider's ownership safeguards.
				subaccount := &accountv1alpha1.Subaccount{ObjectMeta: metav1.ObjectMeta{Name: subaccountResourceName}}
				subaccountErr := deleteCredentialAdoptionTestResources(cleanupCtx, kube, []client.Object{subaccount}, t.Logf, wait.WithTimeout(8*time.Minute))
				if credentialErr != nil || subaccountErr != nil {
					t.Errorf("BTP cleanup unconfirmed: credentials=%v; subaccount=%v; subaccount MR=%q credential name=%q identity=%q. Cluster destruction does not remove BTP resources; confirm external cleanup before rerunning", credentialErr, subaccountErr, subaccountResourceName, credentialName, externalName)
				}
			})
			subaccount := &accountv1alpha1.Subaccount{
				ObjectMeta: metav1.ObjectMeta{Name: subaccountResourceName},
				Spec: accountv1alpha1.SubaccountSpec{
					ForProvider: accountv1alpha1.SubaccountParameters{
						DisplayName:      BUILD_ID + "-subaccount-sac-orphan-import-" + subaccountScenario,
						Region:           "eu10",
						Subdomain:        BUILD_ID + "-e2e-sac-orphan-import-" + subaccountScenario,
						Labels:           map[string]accountv1alpha1.SubaccountLabelValueList{"safe-to-delete": {"yes"}, "BUILD_ID": {BUILD_ID}},
						SubaccountAdmins: []string{envvar.GetOrPanic(TECHNICAL_USER_EMAIL_ENV_KEY)},
					},
				},
			}
			if err := cfg.Client().Resources().Create(ctx, subaccount); err != nil {
				t.Fatalf("create scenario-specific subaccount: %v", err)
			}
			waitForResource(subaccount, cfg, t, wait.WithTimeout(12*time.Minute))

			first := newOrphanRedeployCredential(firstName, firstSecret, cfg.Namespace(), credentialName, subaccountResourceName, xpv1.DeletionOrphan)
			if err := cfg.Client().Resources().Create(ctx, first); err != nil {
				t.Fatalf("create initial credential: %v", err)
			}
			waitForResource(first, cfg, t, wait.WithTimeout(8*time.Minute))
			created := &v1alpha1.SubaccountApiCredential{}
			MustGetResource(t, cfg, firstName, nil, created)
			assertOrphanRedeployCredentialSecret(t, ctx, cfg, created, firstSecret)
			externalName = xpmeta.GetExternalName(created)
			if created.Spec.ForProvider.SubaccountID == nil || externalName != *created.Spec.ForProvider.SubaccountID+"/"+credentialName {
				t.Fatalf("initial external-name %q does not match the created identity", externalName)
			}
			return ctx
		}).
		Assess("Reject annotated adoption and preserve the credential during cleanup "+scenario, func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			first := &v1alpha1.SubaccountApiCredential{}
			MustGetResource(t, cfg, firstName, nil, first)
			AwaitResourceDeletionOrFail(ctx, t, cfg, first, wait.WithTimeout(5*time.Minute))
			if restartProvider {
				restartBTPProvider(t, ctx, cfg)
			}

			// No copied Secret or status: this new MR requests the same identity
			// and supplies its external-name annotation, but has no ownership evidence.
			err := cfg.Client().Resources().Get(ctx, secondSecret, cfg.Namespace(), &corev1.Secret{})
			if !apierrors.IsNotFound(err) {
				t.Fatalf("expected fresh adoption Secret destination to be absent, got %v", err)
			}
			second := newOrphanRedeployCredential(secondName, secondSecret, cfg.Namespace(), credentialName, subaccountResourceName, xpv1.DeletionDelete)
			xpmeta.SetExternalName(second, externalName)
			if err := cfg.Client().Resources().Create(ctx, second); err != nil {
				t.Fatalf("create credential with supplied external-name: %v", err)
			}
			// Do not accept arbitrary transient initializer errors as rejection.
			// Publication is also a terminal observation for this wait: adoption
			// may publish incomplete data, then get stuck in the missing-secret
			// initializer without ever persisting Ready=True. The assertions below
			// must fail in that case, rather than waiting eight minutes for Ready.
			// Collision/missing-secret errors terminate the wait for diagnostics;
			// only the explicit adoption error satisfies the assertion below.
			err = wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(second, func(object k8s.Object) bool {
				got := object.(*v1alpha1.SubaccountApiCredential)
				secret := &corev1.Secret{}
				if secretErr := cfg.Client().Resources().Get(ctx, secondSecret, cfg.Namespace(), secret); secretErr == nil {
					for _, key := range []string{"attribute.client_id", "attribute.client_secret", "attribute.api_url", "attribute.token_url"} {
						if _, published := secret.Data[key]; published {
							return true
						}
					}
				}
				synced := got.GetCondition(xpv1.TypeSynced)
				message := strings.ToLower(synced.Message)
				return got.GetCondition(xpv1.TypeReady).Status == corev1.ConditionTrue ||
					(synced.Status == corev1.ConditionFalse && synced.ObservedGeneration >= got.Generation &&
						(strings.Contains(message, "adopt") || strings.Contains(message, "already exists") ||
							strings.Contains(message, "cannot read client_secret from source")))
			}), wait.WithTimeout(8*time.Minute), wait.WithContext(ctx))
			observed := &v1alpha1.SubaccountApiCredential{}
			if getErr := cfg.Client().Resources().Get(ctx, secondName, "", observed); getErr == nil {
				ready := observed.GetCondition(xpv1.TypeReady)
				synced := observed.GetCondition(xpv1.TypeSynced)
				t.Logf("Annotated MR: generation=%d Ready=%s/%s Synced=%s/%s observedGeneration=%d message=%q finalizers=%v",
					observed.Generation, ready.Status, ready.Reason, synced.Status, synced.Reason,
					synced.ObservedGeneration, synced.Message, observed.Finalizers)
			} else {
				t.Logf("Get annotated MR for diagnostics: %v", getErr)
			}
			if err != nil {
				assertCredentialAdoptionRejectedWithExternalName(t, ctx, cfg, secondName, secondSecret, externalName)
				t.Fatalf("annotated resource neither became Ready, published connection data, nor reported adoption/collision/missing-secret error: %v", err)
			}
			assertCredentialAdoptionRejectedWithExternalName(t, ctx, cfg, secondName, secondSecret, externalName)

			// Even if the readiness assertion failed, exercise cleanup against
			// this isolated, test-owned credential to detect destructive adoption.
			if restartProvider {
				restartBTPProvider(t, ctx, cfg)
			}
			second = &v1alpha1.SubaccountApiCredential{}
			MustGetResource(t, cfg, secondName, nil, second)
			AwaitResourceDeletionOrFail(ctx, t, cfg, second, wait.WithTimeout(5*time.Minute))

			// Require direct evidence that the BTP target still exists. Depending
			// on the provider path, the guarded Read may reject it before Create,
			// or Create may return a duplicate-name error. A generic Synced=False
			// is insufficient; Ready means cleanup lost it and created a replacement.
			probe := newOrphanRedeployCredential(probeName, probeSecret, cfg.Namespace(), credentialName, subaccountResourceName, xpv1.DeletionOrphan)
			if err := cfg.Client().Resources().Create(ctx, probe); err != nil {
				t.Fatalf("create post-cleanup collision probe: %v", err)
			}
			err = wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(probe, func(object k8s.Object) bool {
				got := object.(*v1alpha1.SubaccountApiCredential)
				synced := got.GetCondition(xpv1.TypeSynced)
				return got.GetCondition(xpv1.TypeReady).Status == corev1.ConditionTrue ||
					(synced.Status == corev1.ConditionFalse && synced.ObservedGeneration >= got.Generation &&
						credentialExistenceProof(synced.Message))
			}), wait.WithTimeout(8*time.Minute), wait.WithContext(ctx))
			if err != nil {
				t.Fatalf("post-cleanup probe neither became Ready nor confirmed the existing target: %v", err)
			}
			assertCredentialCollisionRejected(t, ctx, cfg, probeName, probeSecret)
			MustGetResource(t, cfg, probeName, nil, probe)
			if !credentialExistenceProof(probe.GetCondition(xpv1.TypeSynced).Message) {
				t.Errorf("post-cleanup probe did not confirm the original credential still exists; got %q", probe.GetCondition(xpv1.TypeSynced).Message)
			}
			AwaitResourceDeletionOrFail(ctx, t, cfg, probe, wait.WithTimeout(5*time.Minute))
			return ctx
		}).Feature()

	testenv.Test(t, feature)
}

// restartBTPProvider restarts the single provider pod installed by TestMain.
// Identify it by its package image rather than assuming Crossplane's generated
// Deployment labels, which vary across Crossplane versions.
func restartBTPProvider(t *testing.T, ctx context.Context, cfg *envconf.Config) {
	t.Helper()
	r, err := res.New(cfg.Client().RESTConfig())
	if err != nil {
		t.Fatalf("create client for provider restart: %v", err)
	}
	r = r.WithNamespace(xpenvfuncs.CrossplaneNamespace)
	isBTPProvider := func(pod *corev1.Pod) bool {
		for _, container := range pod.Spec.Containers {
			if strings.Contains(container.Image, "/provider-btp-") {
				return true
			}
		}
		return false
	}
	listProviderPods := func() (*corev1.PodList, error) {
		all := &corev1.PodList{}
		if err := r.List(ctx, all); err != nil {
			return nil, err
		}
		matches := &corev1.PodList{}
		for _, pod := range all.Items {
			if isBTPProvider(&pod) {
				matches.Items = append(matches.Items, pod)
			}
		}
		return matches, nil
	}
	pods, err := listProviderPods()
	if err != nil {
		t.Fatalf("list BTP provider pods: %v", err)
	}
	if len(pods.Items) != 1 {
		t.Fatalf("expected one BTP provider pod before restart, got %d", len(pods.Items))
	}
	oldPod := &pods.Items[0]
	oldUID := oldPod.UID
	t.Logf("Restarting BTP provider pod %s (UID %s)", oldPod.Name, oldUID)
	if err := r.Delete(ctx, oldPod, func(options *metav1.DeleteOptions) {
		options.Preconditions = &metav1.Preconditions{UID: &oldUID}
	}); err != nil {
		t.Fatalf("delete BTP provider pod: %v", err)
	}
	if err := wait.For(conditions.New(r).ResourceDeleted(oldPod), wait.WithTimeout(5*time.Minute), wait.WithContext(ctx)); err != nil {
		t.Fatalf("wait for old BTP provider pod to disappear: %v", err)
	}
	if err := wait.For(func(ctx context.Context) (bool, error) {
		current, err := listProviderPods()
		if err != nil {
			return false, nil
		}
		for i := range current.Items {
			pod := &current.Items[i]
			if pod.UID == oldUID || !pod.DeletionTimestamp.IsZero() {
				continue
			}
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					t.Logf("Replacement BTP provider pod %s is Ready (UID %s)", pod.Name, pod.UID)
					return true, nil
				}
			}
		}
		return false, nil
	}, wait.WithTimeout(5*time.Minute), wait.WithContext(ctx)); err != nil {
		t.Fatalf("wait for replacement BTP provider pod to become Ready: %v", err)
	}
}

func newOrphanRedeployCredential(resourceName, secretName, namespace, credentialName, subaccountName string, deletionPolicy xpv1.DeletionPolicy) *v1alpha1.SubaccountApiCredential {
	readOnly := false
	return &v1alpha1.SubaccountApiCredential{
		ObjectMeta: metav1.ObjectMeta{Name: resourceName},
		Spec: v1alpha1.SubaccountApiCredentialSpec{
			ClusterManagedResourceSpec: xpv1.ClusterManagedResourceSpec{
				DeletionPolicy: deletionPolicy,
				WriteConnectionSecretToReference: &xpv1.SecretReference{
					Name: secretName, Namespace: namespace,
				},
			},
			ForProvider: v1alpha1.SubaccountApiCredentialParameters{
				Name:          &credentialName,
				ReadOnly:      &readOnly,
				SubaccountRef: &xpv1.Reference{Name: subaccountName},
			},
		},
	}
}

func assertCredentialCollisionRejected(t *testing.T, ctx context.Context, cfg *envconf.Config, name, secretName string) {
	t.Helper()
	assertCredentialRejected(t, ctx, cfg, name, secretName, "", false)
	cr := &v1alpha1.SubaccountApiCredential{}
	if err := cfg.Client().Resources().Get(ctx, name, "", cr); err != nil {
		t.Errorf("get post-cleanup collision probe %q: %v", name, err)
	} else if synced := cr.GetCondition(xpv1.TypeSynced); !credentialExistenceProof(synced.Message) {
		t.Errorf("post-cleanup probe did not confirm the original credential still exists; got Synced=%s/%s message=%q", synced.Status, synced.Reason, synced.Message)
	}
}

// A supplied annotation may remain on a rejected MR; its presence alone is not
// evidence that the provider claimed ownership. Readiness and publication must
// still be rejected, and the provider must not retarget the annotation.
func assertCredentialAdoptionRejectedWithExternalName(t *testing.T, ctx context.Context, cfg *envconf.Config, name, secretName, suppliedExternalName string) {
	t.Helper()
	assertCredentialRejected(t, ctx, cfg, name, secretName, suppliedExternalName, true)
}

func assertCredentialRejected(t *testing.T, ctx context.Context, cfg *envconf.Config, name, secretName, suppliedExternalName string, requireAdoptionError bool) {
	t.Helper()
	cr := &v1alpha1.SubaccountApiCredential{}
	if err := cfg.Client().Resources().Get(ctx, name, "", cr); err != nil {
		t.Errorf("get rejected resource %q: %v", name, err)
	} else if err := credentialRejectionError(cr, suppliedExternalName, requireAdoptionError); err != nil {
		t.Errorf("resource %q: %v", name, err)
	}

	secret := &corev1.Secret{}
	err := cfg.Client().Resources().Get(ctx, secretName, cfg.Namespace(), secret)
	if err == nil {
		for _, key := range []string{"attribute.client_id", "attribute.client_secret", "attribute.api_url", "attribute.token_url"} {
			if _, ok := secret.Data[key]; ok {
				t.Errorf("rejected resource %q published %q to Secret %q", name, key, secretName)
			}
		}
	} else if !apierrors.IsNotFound(err) {
		t.Errorf("get connection Secret %q: %v", secretName, err)
	}
}

func assertOrphanRedeployCredentialSecret(t *testing.T, ctx context.Context, cfg *envconf.Config, cr *v1alpha1.SubaccountApiCredential, secretName string) {
	t.Helper()
	secret := &corev1.Secret{}
	if err := cfg.Client().Resources().Get(ctx, secretName, cfg.Namespace(), secret); err != nil {
		t.Fatalf("get initial connection Secret %q: %v", secretName, err)
	}
	if len(secret.Data["attribute.client_secret"]) == 0 {
		t.Fatal("initial managed credential Secret has no client_secret")
	}
	if _, ok := secret.Data["attribute.client_id"]; !ok {
		t.Fatal("initial managed credential Secret has no client_id")
	}
	if cr.Spec.ForProvider.Name == nil || *cr.Spec.ForProvider.Name == "" {
		t.Fatal("initial credential does not have an explicit BTP name")
	}
}
