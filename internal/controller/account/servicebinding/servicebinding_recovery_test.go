package servicebinding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeobj "k8s.io/apimachinery/pkg/runtime"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/recovery"
)

var (
	crCreatedAtSB     = time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	createPendingAtSB = crCreatedAtSB.Add(5 * time.Second)
)

// sbLookuperFake is a test double for servicemanager.SemanticLookuper scoped
// to the ServiceBinding heal use case.
type sbLookuperFake struct {
	guid      string
	createdAt time.Time
	found     bool
	err       error
	gotSIID   string
	gotName   string
	calls     int
}

func (l *sbLookuperFake) LookupServiceInstance(ctx context.Context, name string) (string, time.Time, bool, error) {
	return "", time.Time{}, false, nil
}
func (l *sbLookuperFake) LookupServiceBinding(ctx context.Context, serviceInstanceID, name string) (string, time.Time, bool, error) {
	l.calls++
	l.gotSIID = serviceInstanceID
	l.gotName = name
	return l.guid, l.createdAt, l.found, l.err
}
func (l *sbLookuperFake) LookupInstanceAndBinding(ctx context.Context, planID, instanceName, bindingName string) (string, string, time.Time, bool, error) {
	return "", "", time.Time{}, false, nil
}

type sbRecorderFake struct{ events []string }

func (r *sbRecorderFake) Event(_ runtimeobj.Object, e event.Event) {
	r.events = append(r.events, string(e.Reason))
}
func (r *sbRecorderFake) WithAnnotations(_ ...string) event.Recorder { return r }
func (r *sbRecorderFake) has(reason string) bool {
	for _, e := range r.events {
		if e == reason {
			return true
		}
	}
	return false
}

func sbFactory(lk *sbLookuperFake) func(context.Context, *v1alpha1.ServiceBinding) (smClient.SemanticLookuper, func(), error) {
	return func(context.Context, *v1alpha1.ServiceBinding) (smClient.SemanticLookuper, func(), error) {
		return lk, func() {}, nil
	}
}

// sbForAdoption builds a ServiceBinding CR ready for the heal path:
//   - CR-name-fallback external-name (== metadata.name)
//   - non-zero external-create-pending annotation (so the ownership check
//     has a reference point; without it the heal short-circuits)
//   - resolved parent instance ID and binding name in spec
func sbForAdoption(name, siID, bindingName string) *v1alpha1.ServiceBinding {
	cr := &v1alpha1.ServiceBinding{}
	cr.SetName(name)
	cr.SetCreationTimestamp(metav1.NewTime(crCreatedAtSB))
	meta.SetExternalCreatePending(cr, createPendingAtSB)
	meta.SetExternalName(cr, name) // fallback external-name == metadata.name
	cr.Spec.ForProvider.Name = bindingName
	cr.Spec.ForProvider.ServiceInstanceID = internal.Ptr(siID)
	return cr
}

// sbForAdoptionNoPending mirrors sbForAdoption but leaves off the
// external-create-pending annotation \u2014 Create was never attempted.
func sbForAdoptionNoPending(name, siID, bindingName string) *v1alpha1.ServiceBinding {
	cr := sbForAdoption(name, siID, bindingName)
	delete(cr.GetAnnotations(), "crossplane.io/external-create-pending")
	return cr
}

// notExistingObs makes the underlying SB client report ResourceExists=false,
// which drives the heal branch in Observe.
func notExistingObs() managed.ExternalObservation {
	return managed.ExternalObservation{ResourceExists: false}
}

func newExternalForSBAdopt(lk *sbLookuperFake, rec event.Recorder) external {
	factory := &MockServiceBindingClientFactory{
		Client: &MockServiceBindingClient{observation: notExistingObs()},
	}
	return external{
		kube: &test.MockClient{
			MockUpdate:       test.NewMockUpdateFn(nil),
			MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
		},
		clientFactory:      factory,
		client:             factory.Client,
		newAdminLookuperFn: sbFactory(lk),
		recorder:           rec,
	}
}

func TestObserve_ServiceBindingAdoption(t *testing.T) {
	const guid = "80540c06-2955-4bce-9c43-ad78fecc7f62"

	t.Run("match adopts external-name and requeues", func(t *testing.T) {
		cr := sbForAdoption("sb-1", "si-1", "binding-1")
		lk := &sbLookuperFake{guid: guid, createdAt: createPendingAtSB.Add(2 * time.Second), found: true}
		e := newExternalForSBAdopt(lk, nil)

		_, err := e.Observe(context.TODO(), cr)
		if !errors.Is(err, recovery.ErrRequeueAfterRecovery) {
			t.Fatalf("expected ErrRequeueAfterAdopt, got %v", err)
		}
		if got := meta.GetExternalName(cr); got != guid {
			t.Errorf("external-name = %q, want %q", got, guid)
		}
		if lk.gotSIID != "si-1" {
			t.Errorf("lookup siID = %q, want si-1", lk.gotSIID)
		}
		if lk.gotName != "binding-1" {
			t.Errorf("lookup name = %q, want binding-1", lk.gotName)
		}
	})

	// SB-specific: without a resolved parent instance ID we cannot narrow the
	// lookup to a subaccount, so the heal must silently no-op instead of
	// running a global search.
	t.Run("empty serviceInstanceID: no-op, does not run lookup", func(t *testing.T) {
		cr := sbForAdoption("sb-nosi", "", "binding-x")
		lk := &sbLookuperFake{guid: guid, createdAt: createPendingAtSB.Add(2 * time.Second), found: true}
		e := newExternalForSBAdopt(lk, nil)

		obs, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if obs.ResourceExists {
			t.Errorf("expected ResourceExists=false")
		}
		if got := meta.GetExternalName(cr); got != "sb-nosi" {
			t.Errorf("external-name must be unchanged, got %q", got)
		}
		if lk.calls != 0 {
			t.Errorf("lookup must NOT run when serviceInstanceID is empty, got calls=%d", lk.calls)
		}
	})

	// SB-specific: rotation writes the currently-active binding name onto
	// status.atProvider.Name. The heal prefers that over spec so the semantic
	// lookup targets the freshly-rotated binding rather than the spec's
	// original name.
	t.Run("rotation: status.AtProvider.Name overrides spec name", func(t *testing.T) {
		cr := sbForAdoption("sb-rot", "si-rot", "spec-name")
		cr.Status.AtProvider.Name = "rotated-name-abc"
		lk := &sbLookuperFake{guid: guid, createdAt: createPendingAtSB.Add(2 * time.Second), found: true}
		e := newExternalForSBAdopt(lk, nil)

		_, err := e.Observe(context.TODO(), cr)
		if !errors.Is(err, recovery.ErrRequeueAfterRecovery) {
			t.Fatalf("expected ErrRequeueAfterAdopt, got %v", err)
		}
		if lk.gotName != "rotated-name-abc" {
			t.Errorf("lookup name = %q, want rotated-name-abc (status.AtProvider.Name)", lk.gotName)
		}
	})

	t.Run("no match returns ResourceExists=false and does not patch", func(t *testing.T) {
		cr := sbForAdoption("sb-2", "si-2", "binding-2")
		lk := &sbLookuperFake{found: false}
		e := newExternalForSBAdopt(lk, nil)

		obs, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if obs.ResourceExists {
			t.Errorf("expected ResourceExists=false")
		}
		if got := meta.GetExternalName(cr); got != "sb-2" {
			t.Errorf("external-name must be unchanged, got %q", got)
		}
	})

	t.Run("lookup error emits Warning and leaves external-name untouched", func(t *testing.T) {
		cr := sbForAdoption("sb-3", "si-3", "binding-3")
		lk := &sbLookuperFake{err: errors.New("boom")}
		rec := &sbRecorderFake{}
		e := newExternalForSBAdopt(lk, rec)

		_, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error (best-effort), got %v", err)
		}
		if got := meta.GetExternalName(cr); got != "sb-3" {
			t.Errorf("external-name must be unchanged, got %q", got)
		}
		if !rec.has(recovery.EventReasonLookupFailed) {
			t.Errorf("expected %q event, got %+v", recovery.EventReasonLookupFailed, rec.events)
		}
		if rec.has(recovery.EventReasonRecovered) {
			t.Errorf("must not record %q on lookup failure", recovery.EventReasonRecovered)
		}
	})

	// Regression: ownership check refuses to adopt a binding whose BTP
	// created_at falls outside the window around our recorded Create attempt.
	t.Run("brownfield (BTP created outside pending window): refuses adoption, emits Warning", func(t *testing.T) {
		cr := sbForAdoption("sb-brown", "si-brown", "binding-brown")
		lk := &sbLookuperFake{guid: guid, createdAt: createPendingAtSB.Add(-time.Hour), found: true}
		rec := &sbRecorderFake{}
		e := newExternalForSBAdopt(lk, rec)

		_, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error (adoption declined), got %v", err)
		}
		if got := meta.GetExternalName(cr); got != "sb-brown" {
			t.Errorf("external-name must be unchanged, got %q", got)
		}
		if !rec.has(recovery.EventReasonRefusedBrownfield) {
			t.Errorf("expected %q event, got %+v", recovery.EventReasonRefusedBrownfield, rec.events)
		}
		if rec.has(recovery.EventReasonRecovered) {
			t.Errorf("must not record %q when refusing brownfield", recovery.EventReasonRecovered)
		}
	})

	// Regression: no external-create-pending annotation means this controller
	// never attempted Create() for this CR. The heal must short-circuit BEFORE
	// running the expensive semantic lookup.
	t.Run("no create-pending annotation: short-circuits, does not lookup", func(t *testing.T) {
		cr := sbForAdoptionNoPending("sb-nopending", "si-nopending", "binding-nopending")
		lk := &sbLookuperFake{guid: guid, createdAt: createPendingAtSB.Add(2 * time.Second), found: true}
		e := newExternalForSBAdopt(lk, nil)

		obs, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if obs.ResourceExists {
			t.Errorf("expected ResourceExists=false")
		}
		if got := meta.GetExternalName(cr); got != "sb-nopending" {
			t.Errorf("external-name must be unchanged, got %q", got)
		}
		if lk.calls != 0 {
			t.Errorf("lookup must not run when Create has never been attempted, got calls=%d", lk.calls)
		}
	})
}
