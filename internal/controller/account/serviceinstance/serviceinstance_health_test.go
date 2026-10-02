package serviceinstance

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	ujresource "github.com/crossplane/upjet/v2/pkg/resource"
	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const healthTestID = "550e8400-e29b-41d4-a716-446655440000"

func TestObserveExternalHealth(t *testing.T) {
	cases := map[string]struct {
		state       string
		ready       *bool
		usable      *bool
		deleting    bool
		observeOnly bool
		wantStatus  corev1.ConditionStatus
		wantReason  xpv1.ConditionReason
	}{
		"FailedWithoutFlags":  {state: "failed", wantStatus: corev1.ConditionFalse, wantReason: "ExternalResourceFailed"},
		"FailedWithTrueFlags": {state: "FAILED", ready: internal.Ptr(true), usable: internal.Ptr(true), wantStatus: corev1.ConditionFalse, wantReason: "ExternalResourceFailed"},
		"ReadyFalse":          {state: "in progress", ready: internal.Ptr(false), usable: internal.Ptr(true), wantStatus: corev1.ConditionFalse, wantReason: xpv1.ReasonUnavailable},
		"UsableFalse":         {state: "succeeded", ready: internal.Ptr(true), usable: internal.Ptr(false), wantStatus: corev1.ConditionFalse, wantReason: xpv1.ReasonUnavailable},
		"FlagsNotReported":    {state: "succeeded", wantStatus: corev1.ConditionTrue, wantReason: xpv1.ReasonAvailable},
		"Healthy":             {state: "succeeded", ready: internal.Ptr(true), usable: internal.Ptr(true), wantStatus: corev1.ConditionTrue, wantReason: xpv1.ReasonAvailable},
		"DeletingFalseFlags":  {state: "in progress", ready: internal.Ptr(false), usable: internal.Ptr(false), deleting: true, wantStatus: corev1.ConditionTrue, wantReason: xpv1.ReasonAvailable},
		"DeletingFailedState": {state: "failed", deleting: true, wantStatus: corev1.ConditionFalse, wantReason: "ExternalResourceFailed"},
		"ObserveOnlyFailed":   {state: "failed", observeOnly: true, wantStatus: corev1.ConditionUnknown},
		"ObserveOnlyHealthy":  {state: "succeeded", ready: internal.Ptr(true), usable: internal.Ptr(true), observeOnly: true, wantStatus: corev1.ConditionUnknown},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := &v1alpha1.ServiceInstance{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
			meta.SetExternalName(cr, healthTestID)
			if tc.deleting {
				cr.DeletionTimestamp = internal.Ptr(metav1.Now())
			}
			if tc.observeOnly {
				cr.Spec.ManagementPolicies = xpv1.ManagementPolicies{xpv1.ManagementActionObserve}
			}
			e := &external{tfClient: &TfProxyMock{
				status:  tfclient.UpToDate,
				data:    &tfclient.ObservationData{ExternalName: healthTestID, ID: healthTestID, State: tc.state, Ready: tc.ready, Usable: tc.usable},
				details: map[string][]byte{"id": []byte(healthTestID)},
			}}
			got, err := e.Observe(context.Background(), cr)
			if err != nil {
				t.Fatal(err)
			}
			if !got.ResourceExists || !got.ResourceUpToDate || string(got.ConnectionDetails["id"]) != healthTestID {
				t.Fatalf("unexpected observation: %#v", got)
			}
			ready := cr.GetCondition(xpv1.TypeReady)
			if ready.Status != tc.wantStatus || (tc.wantReason != "" && ready.Reason != tc.wantReason) {
				t.Errorf("want Ready=%s/%s, got %s/%s", tc.wantStatus, tc.wantReason, ready.Status, ready.Reason)
			}
			if ready.Status == corev1.ConditionFalse {
				if ready.ObservedGeneration != cr.Generation || !strings.Contains(ready.Message, "state=") || !strings.Contains(ready.Message, "ready=") || !strings.Contains(ready.Message, "usable=") {
					t.Errorf("missing observed health context: %#v", ready)
				}
			}
			if cr.Status.AtProvider.State != tc.state {
				t.Errorf("observation was not saved: %#v", cr.Status.AtProvider)
			}
		})
	}
}

// The managed reconciler writes Synced independently of Ready. Exercise its
// status writes and a subsequent reconcile to catch Ready being overwritten.
func TestReconcileExternalHealthAndRecovery(t *testing.T) {
	for _, fresh := range []bool{true, false} {
		name := "LastKnownHealth"
		if fresh {
			name = "RefreshedHealth"
		}
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := apis.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			cr := &v1alpha1.ServiceInstance{ObjectMeta: metav1.ObjectMeta{Name: "instance", Generation: 2}}
			meta.SetExternalName(cr, healthTestID)
			cr.Spec.ManagementPolicies = xpv1.ManagementPolicies{xpv1.ManagementActionAll}
			cr.Status.AtProvider.ID = healthTestID
			cr.Status.AtProvider.State = "failed"
			cr.Status.AtProvider.Ready = internal.Ptr(false)
			cr.Status.AtProvider.Usable = internal.Ptr(false)
			cr.SetConditions(xpv1.Available(), xpv1.ReconcileSuccess(), xpv1.Condition{
				Type: xpv1.ConditionType(ujresource.TypeLastAsyncOperation), Status: corev1.ConditionFalse,
				Reason: ujresource.ReasonApplyFailure, Message: "apply failed: API Conflict", ObservedGeneration: 2,
			})
			kube := kubefake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha1.ServiceInstance{}).WithObjects(cr).Build()
			tf := &TfProxyMock{status: tfclient.UpToDate}
			if fresh {
				tf.data = &tfclient.ObservationData{ExternalName: healthTestID, ID: healthTestID, State: "failed", Ready: internal.Ptr(false), Usable: internal.Ptr(false)}
			}
			e := &external{tfClient: tf, kube: kube}
			r := managed.NewReconciler(&fake.Manager{Client: kube, Scheme: scheme}, resource.ManagedKind(v1alpha1.ServiceInstanceGroupVersionKind),
				managed.WithExternalConnector(managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) { return e, nil })),
				managed.WithInitializers(), managed.WithRecorder(event.NewNopRecorder()))
			key := types.NamespacedName{Name: cr.Name}
			check := func(wantStatus corev1.ConditionStatus, wantReason xpv1.ConditionReason) {
				t.Helper()
				for range 3 {
					if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
						t.Fatal(err)
					}
				}
				got := &v1alpha1.ServiceInstance{}
				if err := kube.Get(context.Background(), key, got); err != nil {
					t.Fatal(err)
				}
				if ready := got.GetCondition(xpv1.TypeReady); ready.Status != wantStatus || ready.Reason != wantReason {
					t.Errorf("want Ready=%s/%s, got %#v", wantStatus, wantReason, ready)
				}
				if synced := got.GetCondition(xpv1.TypeSynced); synced.Status != corev1.ConditionTrue || synced.Reason != xpv1.ReasonReconcileSuccess {
					t.Errorf("successful observation must keep Synced=True: %#v", synced)
				}
			}
			check(corev1.ConditionFalse, "ExternalResourceFailed")
			// A fresh healthy observation supersedes both the failed health and a
			// stale ApplyFailure callback; it does not need a spec change.
			tf.data = &tfclient.ObservationData{ExternalName: healthTestID, ID: healthTestID, State: "succeeded", Ready: internal.Ptr(true), Usable: internal.Ptr(true)}
			check(corev1.ConditionTrue, xpv1.ReasonAvailable)
		})
	}
}

func TestObserveExternalHealthPreservesDriftAndRejectedUpdate(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		name := "Drift"
		if rejected {
			name = "RejectedUpdate"
		}
		t.Run(name, func(t *testing.T) {
			cr := &v1alpha1.ServiceInstance{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
			meta.SetExternalName(cr, healthTestID)
			cr.Status.AtProvider.State = "failed"
			tf := &TfProxyMock{status: tfclient.Drift}
			wantReason := xpv1.ConditionReason("DriftDetected")
			if rejected {
				tf.status = tfclient.UpToDate
				cr.SetConditions(xpv1.Condition{Type: xpv1.ConditionType(ujresource.TypeLastAsyncOperation), Status: corev1.ConditionFalse, Reason: ujresource.ReasonAsyncUpdateFailure, ObservedGeneration: 2})
				wantReason = reasonAsyncOperationFailed
			}
			got, err := (&external{tfClient: tf}).Observe(context.Background(), cr)
			if err != nil {
				t.Fatal(err)
			}
			if !got.ResourceExists || got.ResourceUpToDate {
				t.Fatalf("unexpected observation: %#v", got)
			}
			if ready := cr.GetCondition(xpv1.TypeReady); ready.Status != corev1.ConditionFalse || ready.Reason != wantReason {
				t.Errorf("expected Ready=False/%s, got %#v", wantReason, ready)
			}
		})
	}
}

func TestObserveExternalHealthMessageBound(t *testing.T) {
	cr := &v1alpha1.ServiceInstance{}
	meta.SetExternalName(cr, healthTestID)
	data := &tfclient.ObservationData{ExternalName: healthTestID, State: strings.Repeat("界", 1024), Ready: internal.Ptr(false)}
	if _, err := (&external{tfClient: &TfProxyMock{status: tfclient.UpToDate, data: data}}).Observe(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	message := cr.GetCondition(xpv1.TypeReady).Message
	if len(message) > 1024 || !utf8.ValidString(message) {
		t.Errorf("health message must be bounded and UTF-8 valid: %d bytes, valid=%t", len(message), utf8.ValidString(message))
	}
}
