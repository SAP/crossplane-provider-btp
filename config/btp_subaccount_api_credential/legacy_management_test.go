package btp_subaccount_api_credential

import (
	"context"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLegacyManagementEnrollmentAndApproval(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cr := &securityv1alpha1.SubaccountApiCredential{
		ObjectMeta: metav1.ObjectMeta{Name: "credential", UID: types.UID("legacy-uid")},
	}
	cr.Status.Conditions = []xpv1.Condition{{Type: xpv1.TypeReady, Status: "True"}}
	meta.SetExternalName(cr, "subaccount-id/credential-name")
	kube := kubefake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cr).WithObjects(cr).Build()
	initializer := &LegacyManagementEnrollmentInitializer{Kube: kube}
	if err := initializer.Initialize(context.Background(), cr); err != nil {
		t.Fatalf("enrollment should persist classification before the later guard: %v", err)
	}
	if err := (&LegacyManagementApprovalInitializer{}).Initialize(context.Background(), cr); err == nil || !strings.Contains(err.Error(), errLegacyManagementApproval) {
		t.Fatalf("expected legacy uncertainty before management, got %v", err)
	}
	persisted := &securityv1alpha1.SubaccountApiCredential{}
	if err := kube.Get(context.Background(), types.NamespacedName{Name: cr.Name}, persisted); err != nil {
		t.Fatal(err)
	}
	if !IsLegacyManagementUnresolved(persisted) || lifecycleConditionMatches(persisted, conditionFreshManagement) {
		t.Fatalf("legacy classification not persisted: %#v", persisted.Status.Conditions)
	}

	for name, annotation := range map[string]string{
		"exact identity":   "legacy-uid:subaccount-id/credential-name",
		"wrong UID":        "copied-uid:subaccount-id/credential-name",
		"wrong credential": "legacy-uid:subaccount-id/other",
		"wrong subaccount": "legacy-uid:other/credential-name",
		"boolean":          "true",
		"empty":            "",
	} {
		t.Run(name, func(t *testing.T) {
			candidate := persisted.DeepCopy()
			if candidate.Annotations == nil {
				candidate.Annotations = map[string]string{}
			}
			candidate.Annotations[LegacyManagementApprovalAnnotation] = annotation
			if got := LegacyManagementApproved(candidate); got != (name == "exact identity") {
				sid, credentialName, identityOK := EstablishedCredentialIdentity(candidate)
				t.Fatalf("LegacyManagementApproved() = %v, uid=%q annotation=%q condition=%#v identity=(%q,%q,%v)", got, candidate.UID, candidate.Annotations[LegacyManagementApprovalAnnotation], candidate.GetCondition(xpv1.ConditionType(conditionLegacyManagement)), sid, credentialName, identityOK)
			}
		})
	}

	persisted.Annotations[LegacyManagementApprovalAnnotation] = "legacy-uid:subaccount-id/credential-name"
	if err := initializer.Initialize(context.Background(), persisted); err != nil {
		t.Fatalf("matching approval should resume management: %v", err)
	}
	delete(persisted.Annotations, LegacyManagementApprovalAnnotation)
	if LegacyManagementApproved(persisted) {
		t.Fatal("removing approval must revoke authorization")
	}
	persisted.SetConditions(xpv1.Condition{Type: xpv1.ConditionType("AdoptionRejected"), Status: "True", Message: "legacy-uid:subaccount-id/credential-name"})
	persisted.Annotations[LegacyManagementApprovalAnnotation] = "legacy-uid:subaccount-id/credential-name"
	if LegacyManagementApproved(persisted) {
		t.Fatal("approval must not override a recorded adoption rejection")
	}
}

func TestFreshEnrollmentRemainsFreshAfterObservedState(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cr := &securityv1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: "fresh", UID: types.UID("fresh-uid")}}
	kube := kubefake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cr).WithObjects(cr).Build()
	initializer := &LegacyManagementEnrollmentInitializer{Kube: kube}
	if err := initializer.Initialize(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	cr.Status.Conditions = append(cr.Status.Conditions, xpv1.Condition{Type: xpv1.TypeReady, Status: "True"})
	cr.Status.AtProvider.Name = ptr("credential-name")
	if err := initializer.Initialize(context.Background(), cr); err != nil {
		t.Fatalf("fresh classification must remain durable: %v", err)
	}
	if !lifecycleConditionMatches(cr, conditionFreshManagement) || IsLegacyManagementUnresolved(cr) {
		t.Fatalf("fresh MR was reclassified: %#v", cr.Status.Conditions)
	}
}

func TestEstablishedCredentialIdentityRejectsConflictsAndSupportsInitProvider(t *testing.T) {
	name, sid := "credential-name", "subaccount-id"
	cr := &securityv1alpha1.SubaccountApiCredential{}
	cr.Spec.InitProvider.Name = &name
	cr.Spec.InitProvider.SubaccountID = &sid
	if gotSID, gotName, ok := EstablishedCredentialIdentity(cr); !ok || gotSID != sid || gotName != name {
		t.Fatalf("initProvider identity = (%q, %q, %v)", gotSID, gotName, ok)
	}
	other := "other"
	cr.Spec.ForProvider.Name = &other
	if _, _, ok := EstablishedCredentialIdentity(cr); ok {
		t.Fatal("conflicting forProvider/initProvider identity must not be guessed")
	}
}
