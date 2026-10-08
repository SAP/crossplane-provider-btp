package servicemanager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	apisv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	sm "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestObservePreservesStatusOnFailure(t *testing.T) {
	for _, observationErr := range []error{errors.New("instance read failed"), context.DeadlineExceeded, context.Canceled} {
		t.Run(observationErr.Error(), func(t *testing.T) {
			cr := NewServiceManager("example", WithStatus(apisv1beta1.ServiceManagerObservation{
				ServiceInstanceID: "previous-instance", ServiceBindingID: "previous-binding",
			}), WithConditions(xpv1.Available()))
			before := cr.DeepCopy()
			writes := 0
			e := &external{tfClient: &TfClientFake{observeFn: func() (sm.ResourcesStatus, error) {
				return sm.ResourcesStatus{InstanceID: "partial-instance"}, observationErr
			}}, kube: &test.MockClient{MockStatusUpdate: func(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
				writes++
				return errors.New("status failure that must not mask observation")
			}}}
			_, err := e.Observe(context.Background(), cr)
			if !errors.Is(err, observationErr) {
				t.Fatalf("lost observation error: %v", err)
			}
			if writes != 0 {
				t.Fatalf("failed observation wrote status %d times", writes)
			}
			if diff := cmp.Diff(before, cr); diff != "" {
				t.Fatalf("failed observation mutated resource: %s", diff)
			}
		})
	}
}

func TestObserveStatusWriteUsesReconcileContext(t *testing.T) {
	for _, mode := range []string{"active", "expired", "parent-canceled"} {
		t.Run(mode, func(t *testing.T) {
			parent, cancelParent := context.WithCancel(context.Background())
			defer cancelParent()
			deadline := time.Now().Add(time.Minute)
			if mode == "expired" {
				deadline = time.Now().Add(-time.Minute)
			}
			ctx, cancel := context.WithDeadline(parent, deadline)
			defer cancel()
			if mode == "parent-canceled" {
				cancelParent()
			}
			cr := NewServiceManager("example", WithStatus(apisv1beta1.ServiceManagerObservation{
				ServiceInstanceID: "previous-instance", ServiceBindingID: "previous-binding",
			}), WithConditions(xpv1.Available()))
			before := cr.DeepCopy()
			statusErr := errors.New("API status update failed")
			writes := 0
			e := &external{tfClient: &TfClientFake{observeFn: func() (sm.ResourcesStatus, error) { return sm.ResourcesStatus{}, nil }},
				kube: &test.MockClient{MockStatusUpdate: func(writeCtx context.Context, _ client.Object, _ ...client.SubResourceUpdateOption) error {
					writes++
					if writeCtx != ctx {
						t.Error("status write detached from reconcile context")
					}
					gotDeadline, ok := writeCtx.Deadline()
					if !ok || !gotDeadline.Equal(deadline) {
						t.Error("status write changed reconcile deadline")
					}
					return statusErr
				}}}
			_, err := e.Observe(ctx, cr)
			if mode == "active" {
				if writes != 1 || !errors.Is(err, statusErr) {
					t.Fatalf("successful observation must report status error: writes=%d err=%v", writes, err)
				}
			} else {
				if writes != 0 || !errors.Is(err, ctx.Err()) {
					t.Fatalf("expired/canceled context must skip status write: writes=%d err=%v", writes, err)
				}
				if diff := cmp.Diff(before, cr); diff != "" {
					t.Fatalf("canceled status write mutated resource: %s", diff)
				}
			}
		})
	}
}
