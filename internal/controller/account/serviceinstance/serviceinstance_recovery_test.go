package serviceinstance

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeobj "k8s.io/apimachinery/pkg/runtime"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/recovery"
)

// recorderFake collects event.Reasons for assertions.
type recorderFake struct {
	events []string
}

func (r *recorderFake) Event(obj runtimeobj.Object, e event.Event) {
	r.events = append(r.events, string(e.Reason))
}
func (r *recorderFake) WithAnnotations(_ ...string) event.Recorder { return r }
func (r *recorderFake) has(reason string) bool {
	for _, e := range r.events {
		if e == reason {
			return true
		}
	}
	return false
}

// lookuperFake is a test double for servicemanager.SemanticLookuper.
type lookuperFake struct {
	siGUID      string
	siCreatedAt time.Time
	siFound     bool
	siErr       error
	gotName     string
	calls       int
}

func (l *lookuperFake) LookupServiceInstance(ctx context.Context, name string) (string, time.Time, bool, error) {
	l.calls++
	l.gotName = name
	return l.siGUID, l.siCreatedAt, l.siFound, l.siErr
}

func (l *lookuperFake) LookupServiceBinding(ctx context.Context, serviceInstanceID, name string) (string, time.Time, bool, error) {
	return "", time.Time{}, false, nil
}

func (l *lookuperFake) LookupInstanceAndBinding(ctx context.Context, planID, instanceName, bindingName string) (string, string, time.Time, bool, error) {
	return "", "", time.Time{}, false, nil
}

func mkFactory(lk *lookuperFake) func(context.Context, *v1alpha1.ServiceInstance) (smClient.SemanticLookuper, func(), error) {
	return func(context.Context, *v1alpha1.ServiceInstance) (smClient.SemanticLookuper, func(), error) {
		return lk, func() {}, nil
	}
}

// crCreatedAt is the reference K8s creationTimestamp used by test CRs. The
// lookuperFake defaults its siCreatedAt to a few seconds AFTER the pending
// annotation so ownership checks pass by default; brownfield cases push
// siCreatedAt OUTSIDE the [pending-60s, pending+1h] window.
var (
	crCreatedAt     = time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	createPendingAt = crCreatedAt.Add(5 * time.Second)
)

// siFallback builds a CR with a fallback external-name (== metadata.name) and a
// recorded Create attempt, simulating a lost-ID Create that recovery must heal.
func siFallback(name string) *v1alpha1.ServiceInstance {
	cr := &v1alpha1.ServiceInstance{}
	cr.SetName(name)
	cr.SetCreationTimestamp(metav1.NewTime(crCreatedAt))
	// Stamp external-create-pending to simulate the runtime having invoked
	// Create() for this CR. Without it, the heal short-circuits (see
	// recovery.HasCreateBeenAttempted) and no recovery happens.
	meta.SetExternalCreatePending(cr, createPendingAt)
	cr.Generation = 2
	cr.Spec.ForProvider.Name = name
	meta.SetExternalName(cr, name) // fallback external-name == metadata.name
	return cr
}

// siFallbackNoPending mirrors siFallback but leaves off the
// external-create-pending annotation — no Create() has ever been attempted for
// this CR. The heal must refuse to recover anything.
func siFallbackNoPending(name string) *v1alpha1.ServiceInstance {
	cr := siFallback(name)
	delete(cr.GetAnnotations(), "crossplane.io/external-create-pending")
	return cr
}

func TestObserve_Recovery(t *testing.T) {
	const guid = "80540c06-2955-4bce-9c43-ad78fecc7f62"

	t.Run("match recovers external-name and requeues", func(t *testing.T) {
		cr := siFallback("cls-1")
		lk := &lookuperFake{siGUID: guid, siCreatedAt: createPendingAt.Add(2 * time.Second), siFound: true}
		rec := &recorderFake{}
		e := external{
			client: &nativeClientMock{},
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			newAdminLookuperFn: mkFactory(lk),
			recorder:           rec,
		}
		_, err := e.Observe(context.TODO(), cr)
		if !errors.Is(err, recovery.ErrRequeueAfterRecovery) {
			t.Fatalf("expected ErrRequeueAfterRecovery, got %v", err)
		}
		if meta.GetExternalName(cr) != guid {
			t.Errorf("external-name = %q, want %q", meta.GetExternalName(cr), guid)
		}
		if lk.gotName != "cls-1" {
			t.Errorf("lookup name = %q, want cls-1", lk.gotName)
		}
		if !rec.has(recovery.EventReasonRecovered) {
			t.Errorf("expected an %q event to be recorded, got %+v", recovery.EventReasonRecovered, rec.events)
		}
	})

	// Regression: ownership check refuses to recover a BTP resource whose
	// created_at falls outside the window around our recorded Create attempt.
	t.Run("brownfield (BTP created outside pending window): refuses recovery, emits Warning", func(t *testing.T) {
		cr := siFallback("cls-brown")
		lk := &lookuperFake{siGUID: guid, siCreatedAt: createPendingAt.Add(-time.Hour), siFound: true}
		rec := &recorderFake{}
		e := external{
			client: &nativeClientMock{},
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			newAdminLookuperFn: mkFactory(lk),
			recorder:           rec,
		}
		obs, err := e.Observe(context.TODO(), cr)
		// Recovery refused -> no error, resource reported not-existing.
		if err != nil {
			t.Fatalf("expected nil error (recovery refused silently), got %v", err)
		}
		if obs.ResourceExists {
			t.Errorf("expected ResourceExists=false")
		}
		if meta.GetExternalName(cr) != "cls-brown" {
			t.Errorf("external-name must be unchanged, got %q", meta.GetExternalName(cr))
		}
		if !rec.has(recovery.EventReasonRefusedBrownfield) {
			t.Errorf("expected a %q event, got %+v", recovery.EventReasonRefusedBrownfield, rec.events)
		}
		if rec.has(recovery.EventReasonRecovered) {
			t.Errorf("must not record an %q event when refusing brownfield", recovery.EventReasonRecovered)
		}
	})

	t.Run("no match returns not-existing and does not patch", func(t *testing.T) {
		cr := siFallback("cls-2")
		lk := &lookuperFake{siFound: false}
		e := external{
			client:             &nativeClientMock{},
			kube:               &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			newAdminLookuperFn: mkFactory(lk),
		}
		obs, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if obs.ResourceExists {
			t.Errorf("expected ResourceExists=false")
		}
		if meta.GetExternalName(cr) != "cls-2" {
			t.Errorf("external-name must be unchanged, got %q", meta.GetExternalName(cr))
		}
	})

	t.Run("lookup error falls through without patching", func(t *testing.T) {
		cr := siFallback("cls-3")
		lk := &lookuperFake{siErr: errors.New("boom")}
		rec := &recorderFake{}
		e := external{
			client:             &nativeClientMock{},
			kube:               &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			newAdminLookuperFn: mkFactory(lk),
			recorder:           rec,
		}
		_, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error (lookup failure logged, not fatal), got %v", err)
		}
		if meta.GetExternalName(cr) != "cls-3" {
			t.Errorf("external-name must be unchanged, got %q", meta.GetExternalName(cr))
		}
		if !rec.has(recovery.EventReasonLookupFailed) {
			t.Errorf("expected an %q event, got %+v", recovery.EventReasonLookupFailed, rec.events)
		}
		if rec.has(recovery.EventReasonRecovered) {
			t.Errorf("must not record an %q event on lookup failure", recovery.EventReasonRecovered)
		}
	})

	// No external-create-pending annotation means this controller never invoked
	// Create() for this CR, so the heal must short-circuit BEFORE running the
	// expensive semantic lookup.
	t.Run("no create-pending annotation: short-circuits, does not lookup", func(t *testing.T) {
		cr := siFallbackNoPending("cls-nopending")
		lk := &lookuperFake{siGUID: guid, siCreatedAt: createPendingAt.Add(2 * time.Second), siFound: true}
		e := external{
			client:             &nativeClientMock{},
			kube:               &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			newAdminLookuperFn: mkFactory(lk),
		}
		_, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if meta.GetExternalName(cr) != "cls-nopending" {
			t.Errorf("external-name must be unchanged, got %q", meta.GetExternalName(cr))
		}
		if lk.calls != 0 {
			t.Errorf("lookup must not run when Create has never been attempted, got calls=%d", lk.calls)
		}
	})
}
