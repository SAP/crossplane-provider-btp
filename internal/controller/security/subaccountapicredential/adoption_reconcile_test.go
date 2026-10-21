package subaccountapicredential

import (
	"context"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	resourcefake "github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	credentialconfig "github.com/sap/crossplane-provider-btp/config/btp_subaccount_api_credential"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileFinalizesFailedAdoptionWithoutExternalDelete(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	now := metav1.Now()
	cr := &v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{
		Name:              "credential",
		UID:               types.UID("fresh-uid"),
		Finalizers:        []string{managed.FinalizerName},
		DeletionTimestamp: &now,
	}}
	meta.SetExternalName(cr, "subaccount-guid/credential-name")
	cr.SetConditions(xpv1.Condition{Type: conditionAdoptionRejected, Status: "True", Message: lifecycleEvidenceMessage(cr)})
	cr.SetConditions(xpv1.Unavailable().WithMessage(errUnsupportedAdoption))
	cr.Spec.WriteConnectionSecretToReference = &xpv1.SecretReference{Name: "credential-secret", Namespace: "default"}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "credential-secret", Namespace: "default"}, Data: map[string][]byte{
		"attribute.client_id": []byte("public-id"),
		"attribute.api_url":   []byte("https://api.example.test"),
		"attribute.token_url": []byte("https://token.example.test"),
	}}
	kube := kubefake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha1.SubaccountApiCredential{}).WithObjects(cr, secret).Build()
	connectCalls, deleteCalls := 0, 0
	inner := managed.ExternalClientFns{
		DeleteFn: func(context.Context, resource.Managed) (managed.ExternalDelete, error) {
			deleteCalls++
			return managed.ExternalDelete{}, nil
		},
		DisconnectFn: func(context.Context) error { return nil },
	}
	connector := managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) {
		connectCalls++
		return inner, nil
	})
	r := managed.NewReconciler(
		&resourcefake.Manager{Client: kube, Scheme: scheme},
		resource.ManagedKind(v1alpha1.SubaccountApiCredential_GroupVersionKind),
		managed.WithExternalConnector(&adoptionRejectingConnector{ExternalConnector: connector}),
		managed.WithInitializers(managed.InitializerChain{
			&credentialconfig.DeletionProtectionInitializer{Kube: kube},
		}),
		managed.WithFinalizer(resource.NewAPIFinalizer(kube, managed.FinalizerName)),
		managed.WithRecorder(event.NewNopRecorder()),
	)
	key := types.NamespacedName{Name: cr.Name}
	for range 5 {
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
			t.Fatalf("Reconcile() error = %v", err)
		}
	}
	got := &v1alpha1.SubaccountApiCredential{}
	if err := kube.Get(context.Background(), key, got); !apierrors.IsNotFound(err) {
		t.Fatalf("failed-adoption MR should be finalized locally; Get() error = %v", err)
	}
	if connectCalls != 0 || deleteCalls != 0 {
		t.Fatalf("cleanup must not connect to or delete BTP credential: connects=%d deletes=%d", connectCalls, deleteCalls)
	}
}

func TestReconcileAllowsObserveAfterSuccessfulCreate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cr := &v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: "credential", UID: types.UID("created-uid")}}
	meta.SetExternalName(cr, "subaccount-guid/credential-name")
	kube := kubefake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha1.SubaccountApiCredential{}).WithObjects(cr).Build()
	observeCalls, createCalls := 0, 0
	inner := managed.ExternalClientFns{
		ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
			observeCalls++
			return managed.ExternalObservation{ResourceExists: observeCalls > 1, ResourceUpToDate: true}, nil
		},
		CreateFn: func(context.Context, resource.Managed) (managed.ExternalCreation, error) {
			createCalls++
			return managed.ExternalCreation{}, nil
		},
		DisconnectFn: func(context.Context) error { return nil },
	}
	connector := managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) {
		return &adoptionRejectingClient{ExternalClient: inner, Kube: kube}, nil
	})
	r := managed.NewReconciler(
		&resourcefake.Manager{Client: kube, Scheme: scheme},
		resource.ManagedKind(v1alpha1.SubaccountApiCredential_GroupVersionKind),
		managed.WithExternalConnector(connector),
		managed.WithInitializers(),
		managed.WithFinalizer(resource.NewAPIFinalizer(kube, managed.FinalizerName)),
		managed.WithRecorder(event.NewNopRecorder()),
	)
	key := types.NamespacedName{Name: cr.Name}
	for range 5 {
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
			t.Fatalf("Reconcile() error = %v", err)
		}
	}
	got := &v1alpha1.SubaccountApiCredential{}
	if err := kube.Get(context.Background(), key, got); err != nil {
		t.Fatal(err)
	}
	if createCalls != 1 || observeCalls < 2 {
		t.Fatalf("expected one Create followed by Observe, got creates=%d observes=%d", createCalls, observeCalls)
	}
	if meta.GetExternalCreateSucceeded(got).IsZero() {
		t.Fatal("managed reconciler did not persist external-create-succeeded after Create")
	}
	if ready := got.GetCondition(xpv1.TypeReady); strings.Contains(ready.Message, errUnsupportedAdoption) {
		t.Fatalf("provider-created credential was falsely rejected as adoption: Ready=%#v conditions=%#v", ready, got.Status.Conditions)
	}
	if synced := got.GetCondition(xpv1.TypeSynced); strings.Contains(synced.Message, errUnsupportedAdoption) {
		t.Fatalf("provider-created credential was falsely rejected as adoption: %#v", synced)
	}
}

func TestReconcileRejectsAdoptionWithoutMutation(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cr := &v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: "credential", UID: types.UID("fresh-uid")}}
	meta.SetExternalName(cr, "subaccount-guid/credential-name")
	kube := kubefake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha1.SubaccountApiCredential{}).WithObjects(cr).Build()
	calls := struct{ observe, create, update, delete int }{}
	inner := managed.ExternalClientFns{
		ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
			calls.observe++
			return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
		},
		CreateFn: func(context.Context, resource.Managed) (managed.ExternalCreation, error) {
			calls.create++
			return managed.ExternalCreation{}, nil
		},
		UpdateFn: func(context.Context, resource.Managed) (managed.ExternalUpdate, error) {
			calls.update++
			return managed.ExternalUpdate{}, nil
		},
		DeleteFn: func(context.Context, resource.Managed) (managed.ExternalDelete, error) {
			calls.delete++
			return managed.ExternalDelete{}, nil
		},
		DisconnectFn: func(context.Context) error { return nil },
	}
	connector := managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) {
		return &adoptionRejectingClient{ExternalClient: inner}, nil
	})
	r := managed.NewReconciler(
		&resourcefake.Manager{Client: kube, Scheme: scheme},
		resource.ManagedKind(v1alpha1.SubaccountApiCredential_GroupVersionKind),
		managed.WithExternalConnector(connector),
		managed.WithInitializers(),
		managed.WithFinalizer(resource.NewAPIFinalizer(kube, managed.FinalizerName)),
		managed.WithRecorder(event.NewNopRecorder()),
	)
	key := types.NamespacedName{Name: cr.Name}
	for range 4 {
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
			t.Fatal(err)
		}
	}
	got := &v1alpha1.SubaccountApiCredential{}
	if err := kube.Get(context.Background(), key, got); err != nil {
		t.Fatal(err)
	}
	if ready := got.GetCondition(xpv1.TypeReady); ready.Status != "False" || !strings.Contains(ready.Message, errUnsupportedAdoption) {
		t.Errorf("expected persisted Ready=False adoption diagnostic, got %#v", ready)
	}
	if synced := got.GetCondition(xpv1.TypeSynced); synced.Status != "False" {
		t.Errorf("expected Synced=False, got %#v", synced)
	}
	if calls.observe == 0 || calls.create != 0 || calls.update != 0 || calls.delete != 0 {
		t.Errorf("unexpected external operations: %#v", calls)
	}
}
