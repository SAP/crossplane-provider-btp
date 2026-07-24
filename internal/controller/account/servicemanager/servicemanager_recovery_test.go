package servicemanager

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

	apisv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	sm "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/recovery"
)

// crCreatedAtSM is a fixed reference for the K8s CR creationTimestamp used by
// the test CRs. The pending Create-attempt (createPendingAtSM) is what the
// ownership check keys off; siCreatedAt values in individual tests are
// relative to the pending time.
var (
	crCreatedAtSM     = time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	createPendingAtSM = crCreatedAtSM.Add(5 * time.Second)
)

// smLookuperFake is a test double for servicemanager.SemanticLookuper scoped
// to the ServiceManager use case (LookupInstanceAndBinding).
type smLookuperFake struct {
	siID        string
	sbID        string
	siCreatedAt time.Time
	found       bool
	err         error
	gotPlan     string
	gotSI       string
	gotSB       string
}

func (l *smLookuperFake) LookupServiceInstance(ctx context.Context, name string) (string, time.Time, bool, error) {
	return "", time.Time{}, false, nil
}
func (l *smLookuperFake) LookupServiceBinding(ctx context.Context, serviceInstanceID, name string) (string, time.Time, bool, error) {
	return "", time.Time{}, false, nil
}
func (l *smLookuperFake) LookupInstanceAndBinding(ctx context.Context, planID, instanceName, bindingName string) (string, string, time.Time, bool, error) {
	l.gotPlan = planID
	l.gotSI = instanceName
	l.gotSB = bindingName
	return l.siID, l.sbID, l.siCreatedAt, l.found, l.err
}

type smRecorderFake struct{ events []string }

func (r *smRecorderFake) Event(_ runtimeobj.Object, e event.Event) {
	r.events = append(r.events, string(e.Reason))
}
func (r *smRecorderFake) WithAnnotations(_ ...string) event.Recorder { return r }
func (r *smRecorderFake) has(reason string) bool {
	for _, e := range r.events {
		if e == reason {
			return true
		}
	}
	return false
}

func smFactory(lk *smLookuperFake) func(context.Context, *apisv1beta1.ServiceManager) (sm.SemanticLookuper, func(), error) {
	return func(context.Context, *apisv1beta1.ServiceManager) (sm.SemanticLookuper, func(), error) {
		return lk, func() {}, nil
	}
}

// smForAdoption builds a ServiceManager CR ready for the heal path: fallback
// external-name (== metadata.name), a resolved plan ID in status, and a
// non-zero external-create-pending annotation so the ownership check has a
// reference point.
func smForAdoption(name, planID string) *apisv1beta1.ServiceManager {
	cr := NewServiceManager(name)
	cr.SetCreationTimestamp(metav1.NewTime(crCreatedAtSM))
	meta.SetExternalCreatePending(cr, createPendingAtSM)
	// NewServiceManager already sets external-name to name; keep as-is (fallback).
	cr.Status.AtProvider.DataSourceLookup = &apisv1beta1.DataSourceLookup{
		ServiceManagerPlanID: planID,
	}
	return cr
}

// smForAdoptionNoPending mirrors smForAdoption but with NO
// external-create-pending annotation. Use to exercise the short-circuit.
func smForAdoptionNoPending(name, planID string) *apisv1beta1.ServiceManager {
	cr := smForAdoption(name, planID)
	delete(cr.GetAnnotations(), "crossplane.io/external-create-pending")
	return cr
}

func TestObserve_ServiceManagerAdoption(t *testing.T) {
	notExisting := func() (sm.ResourcesStatus, error) {
		return sm.ResourcesStatus{ExternalObservation: managed.ExternalObservation{ResourceExists: false}}, nil
	}

	t.Run("match adopts compound external-name and requeues", func(t *testing.T) {
		cr := smForAdoption("sm-1", "plan-1")
		lk := &smLookuperFake{siID: "si-1", sbID: "sb-1", siCreatedAt: createPendingAtSM.Add(2 * time.Second), found: true}
		e := external{
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			tfClient:           &TfClientFake{observeFn: notExisting},
			newAdminLookuperFn: smFactory(lk),
		}
		_, err := e.Observe(context.TODO(), cr)
		if !errors.Is(err, recovery.ErrRequeueAfterRecovery) {
			t.Fatalf("expected ErrRequeueAfterAdopt, got %v", err)
		}
		if got := meta.GetExternalName(cr); got != "si-1/sb-1" {
			t.Errorf("external-name = %q, want si-1/sb-1", got)
		}
		if lk.gotPlan != "plan-1" {
			t.Errorf("lookup plan = %q, want plan-1", lk.gotPlan)
		}
		// The default instance/binding names must be threaded through so the
		// lookuper never picks up an unrelated instance in the subaccount.
		if lk.gotSI != apisv1beta1.DefaultServiceInstanceName {
			t.Errorf("lookup instance-name = %q, want %q", lk.gotSI, apisv1beta1.DefaultServiceInstanceName)
		}
		if lk.gotSB != apisv1beta1.DefaultServiceBindingName {
			t.Errorf("lookup binding-name = %q, want %q", lk.gotSB, apisv1beta1.DefaultServiceBindingName)
		}
	})

	t.Run("instance without binding yields sID-only external-name", func(t *testing.T) {
		cr := smForAdoption("sm-2", "plan-2")
		lk := &smLookuperFake{siID: "si-2", sbID: "", siCreatedAt: createPendingAtSM.Add(2 * time.Second), found: true}
		e := external{
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			tfClient:           &TfClientFake{observeFn: notExisting},
			newAdminLookuperFn: smFactory(lk),
		}
		_, err := e.Observe(context.TODO(), cr)
		if !errors.Is(err, recovery.ErrRequeueAfterRecovery) {
			t.Fatalf("expected ErrRequeueAfterAdopt, got %v", err)
		}
		if got := meta.GetExternalName(cr); got != "si-2" {
			t.Errorf("external-name = %q, want si-2", got)
		}
	})

	t.Run("no match leaves external-name untouched", func(t *testing.T) {
		cr := smForAdoption("sm-3", "plan-3")
		lk := &smLookuperFake{found: false}
		e := external{
			kube:               &test.MockClient{MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil)},
			tfClient:           &TfClientFake{observeFn: notExisting},
			newAdminLookuperFn: smFactory(lk),
		}
		_, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if got := meta.GetExternalName(cr); got != "sm-3" {
			t.Errorf("external-name must be unchanged, got %q", got)
		}
	})

	t.Run("lookup error emits Warning and leaves external-name untouched", func(t *testing.T) {
		cr := smForAdoption("sm-4", "plan-4")
		lk := &smLookuperFake{err: errors.New("boom")}
		rec := &smRecorderFake{}
		e := external{
			kube:               &test.MockClient{MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil)},
			tfClient:           &TfClientFake{observeFn: notExisting},
			newAdminLookuperFn: smFactory(lk),
			recorder:           rec,
		}
		_, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error (best-effort), got %v", err)
		}
		if got := meta.GetExternalName(cr); got != "sm-4" {
			t.Errorf("external-name must be unchanged, got %q", got)
		}
		if !rec.has(recovery.EventReasonLookupFailed) {
			t.Errorf("expected %q event, got %+v", recovery.EventReasonLookupFailed, rec.events)
		}
		if rec.has(recovery.EventReasonRecovered) {
			t.Errorf("must not record %q on lookup failure", recovery.EventReasonRecovered)
		}
	})

	// Regression: single-UUID external-name is NOT a fallback, so adoption
	// must NOT fire even though the compound scheme would benefit from it.
	// This is the phase-1 output during two-phase Create and used to trap the
	// SM controller in an infinite adoption loop. See internal/recovery/recovery.go.
	t.Run("single-UUID external-name does NOT trigger adoption (phase-1 output)", func(t *testing.T) {
		cr := smForAdoption("sm-5", "plan-5")
		meta.SetExternalName(cr, "80540c06-2955-4bce-9c43-ad78fecc7f62") // real UUID, non-compound
		lk := &smLookuperFake{siID: "must-not-be-used", sbID: "must-not-be-used", found: true}
		e := external{
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			tfClient:           &TfClientFake{observeFn: notExisting},
			newAdminLookuperFn: smFactory(lk),
		}
		_, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error (adoption skipped), got %v", err)
		}
		if got := meta.GetExternalName(cr); got != "80540c06-2955-4bce-9c43-ad78fecc7f62" {
			t.Errorf("external-name must be unchanged (adoption must not run), got %q", got)
		}
		if lk.gotPlan != "" {
			t.Errorf("lookup must NOT have been invoked, got planID=%q", lk.gotPlan)
		}
	})

	// Regression: ownership check refuses to adopt an SM instance whose BTP
	// created_at falls outside the window around our recorded Create attempt.
	t.Run("brownfield (BTP created outside pending window): refuses adoption, emits Warning", func(t *testing.T) {
		cr := smForAdoption("sm-brown", "plan-brown")
		lk := &smLookuperFake{siID: "si-brown", sbID: "sb-brown", siCreatedAt: createPendingAtSM.Add(-time.Hour), found: true}
		rec := &smRecorderFake{}
		e := external{
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			tfClient:           &TfClientFake{observeFn: notExisting},
			newAdminLookuperFn: smFactory(lk),
			recorder:           rec,
		}
		_, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error (adoption declined), got %v", err)
		}
		if got := meta.GetExternalName(cr); got != "sm-brown" {
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
	// never attempted Create() for this CR. Adoption must short-circuit BEFORE
	// running the expensive semantic lookup.
	t.Run("no create-pending annotation: short-circuits, does not lookup", func(t *testing.T) {
		cr := smForAdoptionNoPending("sm-nopending", "plan-nopending")
		lk := &smLookuperFake{siID: "must-not-be-used", sbID: "must-not-be-used", found: true}
		e := external{
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			tfClient:           &TfClientFake{observeFn: notExisting},
			newAdminLookuperFn: smFactory(lk),
		}
		_, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if got := meta.GetExternalName(cr); got != "sm-nopending" {
			t.Errorf("external-name must be unchanged, got %q", got)
		}
		if lk.gotPlan != "" {
			t.Errorf("lookup must NOT be invoked when Create was never attempted, got planID=%q", lk.gotPlan)
		}
	})
}
