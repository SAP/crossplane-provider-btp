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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

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
