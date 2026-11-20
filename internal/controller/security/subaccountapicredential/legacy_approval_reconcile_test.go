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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestLegacyApprovalResumesManagementAfterAnnotationUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cr := &v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{
		Name: "legacy", UID: types.UID("legacy-uid"),
	}}
	meta.SetExternalName(cr, "subaccount-id/credential-name")
	cr.Status.Conditions = []xpv1.Condition{{Type: xpv1.TypeReady, Status: "True"}}
	kube := kubefake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cr).WithObjects(cr).Build()
	observes := 0
	inner := managed.ExternalClientFns{
		ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
			observes++
			return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
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
		managed.WithInitializers(managed.InitializerChain{&credentialconfig.LegacyManagementEnrollmentInitializer{Kube: kube}}),
		managed.WithFinalizer(resource.NewAPIFinalizer(kube, managed.FinalizerName)),
		managed.WithRecorder(event.NewNopRecorder()),
	)
	key := types.NamespacedName{Name: cr.Name}
	_, _ = r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	got := &v1alpha1.SubaccountApiCredential{}
	if err := kube.Get(context.Background(), key, got); err != nil {
		t.Fatal(err)
	}
	if !credentialconfig.IsLegacyManagementUnresolved(got) || observes != 0 {
		t.Fatalf("legacy resource was not held before approval: observes=%d conditions=%#v", observes, got.Status.Conditions)
	}
	if got.Annotations == nil {
		got.Annotations = map[string]string{}
	}
	got.Annotations[credentialconfig.LegacyManagementApprovalAnnotation] = "legacy-uid:subaccount-id/credential-name"
	if err := kube.Update(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
			t.Fatalf("Reconcile() after approval: %v", err)
		}
	}
	if observes == 0 {
		t.Fatal("approved legacy resource did not resume observation")
	}
	if err := kube.Get(context.Background(), key, got); err != nil {
		t.Fatal(err)
	}
	if rejected := got.GetCondition(xpv1.ConditionType(conditionAdoptionRejected)); rejected.Status == "True" {
		t.Fatalf("approval was incorrectly rejected as adoption: %#v", rejected)
	}
	if !credentialconfig.LegacyManagementApproved(got) {
		t.Fatal("approval annotation was not preserved through reconciliation")
	}
	if strings.Contains(got.GetCondition(xpv1.TypeReady).Message, errUnsupportedAdoption) {
		t.Fatalf("legacy management still reports unsupported adoption: %#v", got.GetCondition(xpv1.TypeReady))
	}
}
