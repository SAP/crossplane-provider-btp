package subaccountapicredential

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreatePersistsIntentBeforeProviderCallAndOwnershipAfterSuccess(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := securityv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	const (
		name       = "credential-create-receipt"
		uid        = "credential-create-receipt-uid"
		subaccount = "subaccount-create-receipt"
	)
	credentialName := "created-here"
	subaccountID := subaccount
	cr := &securityv1alpha1.SubaccountApiCredential{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(uid)},
		Spec: securityv1alpha1.SubaccountApiCredentialSpec{ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{
			SubaccountID: &subaccountID, Name: &credentialName,
		}},
		Status: securityv1alpha1.SubaccountApiCredentialStatus{Lifecycle: &securityv1alpha1.SubaccountApiCredentialLifecycleStatus{
			ManagedResourceUID: uid, SubaccountID: subaccount, Name: credentialName,
			Phase: securityv1alpha1.SubaccountApiCredentialPhaseEligible,
		}},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cr).WithObjects(cr).Build()
	calls := 0
	base := managed.ExternalClientFns{
		CreateFn: func(ctx context.Context, _ resource.Managed) (managed.ExternalCreation, error) {
			calls++
			persisted := &securityv1alpha1.SubaccountApiCredential{}
			if err := kube.Get(ctx, client.ObjectKeyFromObject(cr), persisted); err != nil {
				t.Fatalf("read persisted intent: %v", err)
			}
			if persisted.Status.Lifecycle == nil || persisted.Status.Lifecycle.Phase != securityv1alpha1.SubaccountApiCredentialPhaseCreatePending || persisted.Status.Lifecycle.AttemptID == "" {
				t.Fatalf("provider Create ran before durable intent was written: lifecycle=%#v", persisted.Status.Lifecycle)
			}
			return managed.ExternalCreation{}, nil
		},
	}
	guarded := &lifecycleExternalClient{kube: kube, base: base}
	if _, err := guarded.Create(ctx, cr); err != nil {
		t.Fatalf("guarded Create: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one provider Create, got %d", calls)
	}
	persisted := &securityv1alpha1.SubaccountApiCredential{}
	if err := kube.Get(ctx, client.ObjectKeyFromObject(cr), persisted); err != nil {
		t.Fatal(err)
	}
	if got := persisted.Status.Lifecycle; got == nil || got.Phase != securityv1alpha1.SubaccountApiCredentialPhaseOwned || got.Reason != "provider-create-succeeded" || got.AttemptID == "" {
		t.Fatalf("successful provider Create did not persist ownership receipt: %#v", got)
	}
}

func TestAbsentAnnotatedTargetRemainsEligibleForCreation(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := securityv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	subaccountID, credentialName := "subaccount-absent", "credential-absent"
	cr := &securityv1alpha1.SubaccountApiCredential{
		ObjectMeta: metav1.ObjectMeta{Name: "absent-target", UID: types.UID("absent-target-uid"), Annotations: map[string]string{"crossplane.io/external-name": subaccountID + "/" + credentialName}},
		Spec:       securityv1alpha1.SubaccountApiCredentialSpec{ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{SubaccountID: &subaccountID, Name: &credentialName}},
		Status: securityv1alpha1.SubaccountApiCredentialStatus{Lifecycle: &securityv1alpha1.SubaccountApiCredentialLifecycleStatus{
			ManagedResourceUID: "absent-target-uid", SubaccountID: subaccountID, Name: credentialName,
			Phase: securityv1alpha1.SubaccountApiCredentialPhaseEligible,
		}},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cr).WithObjects(cr).Build()
	observes := 0
	base := managed.ExternalClientFns{ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
		observes++
		return managed.ExternalObservation{ResourceExists: false}, nil
	}}
	guarded := &lifecycleExternalClient{kube: kube, base: base}
	observation, err := guarded.Observe(ctx, cr)
	if err != nil {
		t.Fatalf("absent annotated target should not be blanket-rejected: %v", err)
	}
	if observation.ResourceExists {
		t.Fatal("absent target reported as existing")
	}
	if observes != 1 {
		t.Fatalf("expected one underlying existence observation, got %d", observes)
	}
}

func TestOwnedTargetAbsenceInvalidatesDeletionAuthority(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := securityv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	subaccountID, credentialName := "subaccount-owned-absence", "credential-owned-absence"
	cr := &securityv1alpha1.SubaccountApiCredential{
		ObjectMeta: metav1.ObjectMeta{Name: "owned-absence", UID: types.UID("owned-absence-uid")},
		Spec:       securityv1alpha1.SubaccountApiCredentialSpec{ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{SubaccountID: &subaccountID, Name: &credentialName}},
		Status: securityv1alpha1.SubaccountApiCredentialStatus{Lifecycle: &securityv1alpha1.SubaccountApiCredentialLifecycleStatus{
			ManagedResourceUID: "owned-absence-uid", SubaccountID: subaccountID, Name: credentialName,
			Phase: securityv1alpha1.SubaccountApiCredentialPhaseOwned, Reason: "provider-create-succeeded", AttemptID: "attempt", AttemptedAt: &metav1.Time{Time: metav1.Now().Time},
		}},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cr).WithObjects(cr).Build()
	base := managed.ExternalClientFns{ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}}
	guarded := &lifecycleExternalClient{kube: kube, base: base}
	if _, err := guarded.Observe(ctx, cr); err != nil {
		t.Fatalf("observe absent owned target: %v", err)
	}
	persisted := &securityv1alpha1.SubaccountApiCredential{}
	if err := kube.Get(ctx, client.ObjectKeyFromObject(cr), persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Status.Lifecycle.Phase != securityv1alpha1.SubaccountApiCredentialPhaseUncertain || persisted.Status.Lifecycle.Reason != "owned-target-observed-absent" {
		t.Fatalf("absence did not invalidate ownership receipt: %#v", persisted.Status.Lifecycle)
	}
	if err := validateOwnedIdentity(persisted); err == nil {
		t.Fatal("stale ownership receipt still authorizes a replacement target")
	}
}

func TestLegacyObservationIsClassifiedUncertain(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := securityv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	name := "legacy-credential"
	cr := &securityv1alpha1.SubaccountApiCredential{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID("legacy-uid")},
		Status: securityv1alpha1.SubaccountApiCredentialStatus{
			ManagedResourceStatus: xpv1.ManagedResourceStatus{ConditionedStatus: xpv1.ConditionedStatus{Conditions: []xpv1.Condition{{Type: xpv1.TypeReady, Status: corev1.ConditionTrue}}}},
		},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cr).WithObjects(cr).Build()
	connector := &lifecycleConnector{kube: kube}
	phase, err := connector.ensureLifecycle(ctx, cr)
	if err != nil {
		t.Fatalf("classify legacy MR: %v", err)
	}
	if phase != securityv1alpha1.SubaccountApiCredentialPhaseUncertain {
		t.Fatalf("expected Uncertain, got %q", phase)
	}
	persisted := &securityv1alpha1.SubaccountApiCredential{}
	if err := kube.Get(ctx, client.ObjectKeyFromObject(cr), persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Status.Lifecycle == nil || persisted.Status.Lifecycle.ManagedResourceUID != string(cr.UID) {
		t.Fatalf("legacy uncertainty was not persisted and UID-bound: %#v", persisted.Status.Lifecycle)
	}
}

func TestIdentityUsesConfiguredAndDefaultCredentialNames(t *testing.T) {
	subaccountID := "subaccount-name-test"
	configured := "configured-no-suffix"
	for _, tc := range []struct {
		name string
		cr   *securityv1alpha1.SubaccountApiCredential
		want string
	}{
		{name: "configured", cr: &securityv1alpha1.SubaccountApiCredential{Spec: securityv1alpha1.SubaccountApiCredentialSpec{ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{SubaccountID: &subaccountID, Name: &configured}}}, want: configured},
		{name: "default", cr: &securityv1alpha1.SubaccountApiCredential{Spec: securityv1alpha1.SubaccountApiCredentialSpec{ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{SubaccountID: &subaccountID}}}, want: defaultCredentialName},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := identityFor(tc.cr)
			if err != nil {
				t.Fatalf("identityFor: %v", err)
			}
			if got.name != tc.want {
				t.Fatalf("credential name = %q, want %q", got.name, tc.want)
			}
		})
	}
}

func TestIdentityIncludesInitProviderParameters(t *testing.T) {
	subaccountID, credentialName := "init-subaccount", "init-credential"
	cr := &securityv1alpha1.SubaccountApiCredential{Spec: securityv1alpha1.SubaccountApiCredentialSpec{
		InitProvider: securityv1alpha1.SubaccountApiCredentialInitParameters{SubaccountID: &subaccountID, Name: &credentialName},
	}}
	identity, err := identityFor(cr)
	if err != nil {
		t.Fatalf("identityFor initProvider-only spec: %v", err)
	}
	if identity.subaccount != subaccountID || identity.name != credentialName {
		t.Fatalf("effective initProvider identity = %#v, want %q/%q", identity, subaccountID, credentialName)
	}
}

func TestLifecycleReceiptRejectsWrongUIDAndRequiresAuditedLegacyEvidence(t *testing.T) {
	subaccountID, credentialName := "subaccount-owned", "credential-owned"
	cr := &securityv1alpha1.SubaccountApiCredential{
		ObjectMeta: metav1.ObjectMeta{Name: "owned-resource", UID: types.UID("current-uid"), Annotations: map[string]string{"crossplane.io/external-name": subaccountID + "/" + credentialName}},
		Spec:       securityv1alpha1.SubaccountApiCredentialSpec{ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{SubaccountID: &subaccountID, Name: &credentialName}},
		Status: securityv1alpha1.SubaccountApiCredentialStatus{Lifecycle: &securityv1alpha1.SubaccountApiCredentialLifecycleStatus{
			ManagedResourceUID: "copied-uid", SubaccountID: subaccountID, Name: credentialName, Phase: securityv1alpha1.SubaccountApiCredentialPhaseOwned, Reason: "provider-create-succeeded",
		}},
	}
	if err := validateOwnedIdentity(cr); err == nil {
		t.Fatal("receipt copied from another UID authorized ownership")
	}
	cr.Status.Lifecycle.ManagedResourceUID = string(cr.UID)
	cr.Status.Lifecycle.Reason = "administrator-resolved-legacy"
	if err := validateOwnedIdentity(cr); err == nil {
		t.Fatal("legacy resolution without audit metadata authorized ownership")
	}
	cr.Status.Lifecycle.ResolvedBy = "cluster-admin"
	cr.Status.Lifecycle.ResolvedAt = &metav1.Time{Time: metav1.Now().Time}
	cr.Status.Lifecycle.ResolutionEvidence = "audit-record-123"
	if err := validateOwnedIdentity(cr); err != nil {
		t.Fatalf("complete UID-bound audited resolution was rejected: %v", err)
	}
}
