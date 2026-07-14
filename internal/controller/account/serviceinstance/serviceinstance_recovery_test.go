package serviceinstance

import (
	"context"
	"strings"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	ujresource "github.com/crossplane/upjet/v2/pkg/resource"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeobj "k8s.io/apimachinery/pkg/runtime"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	tfclient "github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
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
// siCreatedAt OUTSIDE the [pending-60s, pending+5min] window.
var (
	crCreatedAt     = time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	createPendingAt = crCreatedAt.Add(5 * time.Second)
)

func siWithConflict(name string) *v1alpha1.ServiceInstance {
	cr := &v1alpha1.ServiceInstance{}
	cr.SetName(name)
	cr.SetCreationTimestamp(metav1.NewTime(crCreatedAt))
	// Stamp external-create-pending to simulate the runtime having invoked
	// Create() for this CR. Without it, the heal short-circuits (see
	// recovery.HasCreateBeenAttempted) and no adoption happens.
	meta.SetExternalCreatePending(cr, createPendingAt)
	cr.Generation = 2
	cr.Spec.ForProvider.Name = name
	meta.SetExternalName(cr, name) // fallback external-name == metadata.name
	cr.SetConditions(xpv1.Condition{
		Type:               xpv1.ConditionType(ujresource.TypeLastAsyncOperation),
		Status:             corev1.ConditionFalse,
		Reason:             "ApplyFailure",
		Message:            "apply failed: API Error Creating Resource Service Instance (Subaccount): Conflict",
		ObservedGeneration: 2,
	})
	return cr
}

// siWithConflictNoPending mirrors siWithConflict but leaves off the
// external-create-pending annotation — no Create() has ever been attempted
// for this CR. The heal must refuse to adopt anything.
func siWithConflictNoPending(name string) *v1alpha1.ServiceInstance {
	cr := siWithConflict(name)
	delete(cr.GetAnnotations(), "crossplane.io/external-create-pending")
	return cr
}

func TestObserve_AdoptionConflictBranch(t *testing.T) {
	const guid = "80540c06-2955-4bce-9c43-ad78fecc7f62"

	t.Run("match adopts external-name and requeues", func(t *testing.T) {
		cr := siWithConflict("cls-1")
		lk := &lookuperFake{siGUID: guid, siCreatedAt: createPendingAt.Add(2 * time.Second), siFound: true}
		rec := &recorderFake{}
		e := external{
			tfClient: &TfProxyMock{status: tfclient.NotExisting},
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			newAdminLookuperFn: mkFactory(lk),
			recorder:           rec,
		}
		_, err := e.Observe(context.TODO(), cr)
		if !errors.Is(err, recovery.ErrRequeueAfterRecovery) {
			t.Fatalf("expected ErrRequeueAfterAdopt, got %v", err)
		}
		if meta.GetExternalName(cr) != guid {
			t.Errorf("external-name = %q, want %q", meta.GetExternalName(cr), guid)
		}
		if lk.gotName != "cls-1" {
			t.Errorf("lookup name = %q, want cls-1", lk.gotName)
		}
		// the stale LastAsyncOperation=ApplyFailure must be cleared so the next
		// reconcile does not re-enter the Conflict branch.
		cond := cr.GetCondition(xpv1.ConditionType(ujresource.TypeLastAsyncOperation))
		if cond.Reason == "ApplyFailure" {
			t.Errorf("stale ApplyFailure condition was not cleared")
		}
		// a real ID was resolved -> an ExternalNameAdopted event must be logged.
		if !rec.has(recovery.EventReasonRecovered) {
			t.Errorf("expected an %q event to be recorded, got %+v", recovery.EventReasonRecovered, rec.events)
		}
	})

	// Regression: ownership check refuses to adopt a BTP resource whose
	// created_at falls outside the window around our recorded Create attempt.
	// That is the brownfield case — the user must adopt it explicitly by
	// setting crossplane.io/external-name (per the external-name ADR).
	t.Run("brownfield (BTP created outside pending window): refuses adoption, emits Warning", func(t *testing.T) {
		cr := siWithConflict("cls-brown")
		// BTP instance is 1h OLDER than our pending annotation -> outside window -> refuse.
		lk := &lookuperFake{siGUID: guid, siCreatedAt: createPendingAt.Add(-time.Hour), siFound: true}
		rec := &recorderFake{}
		e := external{
			tfClient: &TfProxyMock{status: tfclient.NotExisting},
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			newAdminLookuperFn: mkFactory(lk),
			recorder:           rec,
		}
		_, err := e.Observe(context.TODO(), cr)
		// The Conflict-branch fall-through still returns the "already exists"
		// error (adoption declined, so the original error is preserved).
		if err == nil || errors.Is(err, recovery.ErrRequeueAfterRecovery) {
			t.Fatalf("expected the original conflict error (adoption refused), got %v", err)
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

	t.Run("no match returns the original conflict error and does not patch", func(t *testing.T) {
		cr := siWithConflict("cls-2")
		lk := &lookuperFake{siFound: false}
		e := external{
			tfClient:           &TfProxyMock{status: tfclient.NotExisting},
			kube:               &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			newAdminLookuperFn: mkFactory(lk),
		}
		obs, err := e.Observe(context.TODO(), cr)
		if err == nil || errors.Is(err, recovery.ErrRequeueAfterRecovery) {
			t.Fatalf("expected the original conflict error, got %v", err)
		}
		if obs.ResourceExists {
			t.Errorf("expected ResourceExists=false")
		}
		if meta.GetExternalName(cr) != "cls-2" {
			t.Errorf("external-name must be unchanged, got %q", meta.GetExternalName(cr))
		}
	})

	t.Run("lookup error falls through to original error without patching", func(t *testing.T) {
		cr := siWithConflict("cls-3")
		lk := &lookuperFake{siErr: errors.New("boom")}
		rec := &recorderFake{}
		e := external{
			tfClient:           &TfProxyMock{status: tfclient.NotExisting},
			kube:               &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			newAdminLookuperFn: mkFactory(lk),
			recorder:           rec,
		}
		_, err := e.Observe(context.TODO(), cr)
		if err == nil || errors.Is(err, recovery.ErrRequeueAfterRecovery) {
			t.Fatalf("expected the original conflict error, got %v", err)
		}
		if meta.GetExternalName(cr) != "cls-3" {
			t.Errorf("external-name must be unchanged, got %q", meta.GetExternalName(cr))
		}
		// a lookup failure logs a Warning, never an adoption.
		if !rec.has(recovery.EventReasonLookupFailed) {
			t.Errorf("expected an %q event, got %+v", recovery.EventReasonLookupFailed, rec.events)
		}
		if rec.has(recovery.EventReasonRecovered) {
			t.Errorf("must not record an %q event on lookup failure", recovery.EventReasonRecovered)
		}
	})

	// New: no external-create-pending annotation means this controller never
	// invoked Create() for this CR, so the heal must short-circuit BEFORE
	// running the expensive semantic lookup. Guards the safety property that
	// motivated dropping the creationTimestamp fallback.
	t.Run("no create-pending annotation: short-circuits, does not lookup", func(t *testing.T) {
		cr := siWithConflictNoPending("cls-nopending")
		lk := &lookuperFake{siGUID: guid, siCreatedAt: createPendingAt.Add(2 * time.Second), siFound: true}
		e := external{
			tfClient:           &TfProxyMock{status: tfclient.NotExisting},
			kube:               &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			newAdminLookuperFn: mkFactory(lk),
		}
		_, err := e.Observe(context.TODO(), cr)
		// The Conflict-branch fall-through still returns the "already exists"
		// error; but adoption is refused up-front so the lookup must not run.
		if err == nil || errors.Is(err, recovery.ErrRequeueAfterRecovery) {
			t.Fatalf("expected the original conflict error (adoption refused), got %v", err)
		}
		if meta.GetExternalName(cr) != "cls-nopending" {
			t.Errorf("external-name must be unchanged, got %q", meta.GetExternalName(cr))
		}
		if lk.calls != 0 {
			t.Errorf("lookup must not run when Create has never been attempted, got calls=%d", lk.calls)
		}
	})
}

// TestObserve_AdoptionNotExistingBranch covers the plain not-found path (no
// Conflict condition) which also serves the delete leg.
func TestObserve_AdoptionNotExistingBranch(t *testing.T) {
	const guid = "aaaaaaaa-2955-4bce-9c43-ad78fecc7f62"

	cr := &v1alpha1.ServiceInstance{}
	cr.SetName("cls-x")
	cr.SetCreationTimestamp(metav1.NewTime(crCreatedAt))
	meta.SetExternalCreatePending(cr, createPendingAt)
	cr.Spec.ForProvider.Name = "cls-x"
	meta.SetExternalName(cr, "cls-x")
	lk := &lookuperFake{siGUID: guid, siCreatedAt: createPendingAt.Add(2 * time.Second), siFound: true}
	e := external{
		tfClient: &TfProxyMock{status: tfclient.NotExisting},
		kube: &test.MockClient{
			MockUpdate:       test.NewMockUpdateFn(nil),
			MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
		},
		newAdminLookuperFn: mkFactory(lk),
	}
	_, err := e.Observe(context.TODO(), cr)
	if !errors.Is(err, recovery.ErrRequeueAfterRecovery) {
		t.Fatalf("expected ErrRequeueAfterAdopt, got %v", err)
	}
	if meta.GetExternalName(cr) != guid {
		t.Errorf("external-name = %q, want %q", meta.GetExternalName(cr), guid)
	}
}

// TestObserve_AdoptionBrownfieldNotExistingBranch: same as above but a
// brownfield resource — adoption must be refused and external-name unchanged.
func TestObserve_AdoptionBrownfieldNotExistingBranch(t *testing.T) {
	const guid = "aaaaaaaa-2955-4bce-9c43-ad78fecc7f62"

	cr := &v1alpha1.ServiceInstance{}
	cr.SetName("cls-brown-x")
	cr.SetCreationTimestamp(metav1.NewTime(crCreatedAt))
	meta.SetExternalCreatePending(cr, createPendingAt)
	cr.Spec.ForProvider.Name = "cls-brown-x"
	meta.SetExternalName(cr, "cls-brown-x")
	lk := &lookuperFake{siGUID: guid, siCreatedAt: createPendingAt.Add(-time.Hour), siFound: true}
	rec := &recorderFake{}
	e := external{
		tfClient: &TfProxyMock{status: tfclient.NotExisting},
		kube: &test.MockClient{
			MockUpdate:       test.NewMockUpdateFn(nil),
			MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
		},
		newAdminLookuperFn: mkFactory(lk),
		recorder:           rec,
	}
	obs, err := e.Observe(context.TODO(), cr)
	if err != nil {
		t.Fatalf("expected nil error (adoption refused silently on not-existing branch), got %v", err)
	}
	if obs.ResourceExists {
		t.Errorf("expected ResourceExists=false")
	}
	if meta.GetExternalName(cr) != "cls-brown-x" {
		t.Errorf("external-name must be unchanged, got %q", meta.GetExternalName(cr))
	}
	if !rec.has(recovery.EventReasonRefusedBrownfield) {
		t.Errorf("expected a %q event, got %+v", recovery.EventReasonRefusedBrownfield, rec.events)
	}
}

// silence unused import in some builds
var _ = strings.Contains
var _ = managed.ExternalConnector(nil)
