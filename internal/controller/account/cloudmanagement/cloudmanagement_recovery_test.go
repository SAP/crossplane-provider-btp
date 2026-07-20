package cloudmanagement

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

	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	cmclient "github.com/sap/crossplane-provider-btp/internal/clients/cis"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/recovery"
)

// crCreatedAtCM is a fixed reference for the K8s CR creationTimestamp used by
// the test CRs. The pending Create-attempt (createPendingAtCM) is what the
// ownership check keys off; siCreatedAt values in individual tests are
// relative to the pending time.
var (
	crCreatedAtCM     = time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	createPendingAtCM = crCreatedAtCM.Add(5 * time.Second)
)

type cmLookuperFake struct {
	siID        string
	sbID        string
	siCreatedAt time.Time
	found       bool
	err         error
	gotPlan     string
}

func (l *cmLookuperFake) LookupServiceInstance(ctx context.Context, name string) (string, time.Time, bool, error) {
	return "", time.Time{}, false, nil
}
func (l *cmLookuperFake) LookupServiceBinding(ctx context.Context, serviceInstanceID, name string) (string, time.Time, bool, error) {
	return "", time.Time{}, false, nil
}
func (l *cmLookuperFake) LookupInstanceAndBinding(ctx context.Context, planID, instanceName, bindingName string) (string, string, time.Time, bool, error) {
	l.gotPlan = planID
	return l.siID, l.sbID, l.siCreatedAt, l.found, l.err
}

type cmRecorderFake struct{ events []string }

func (r *cmRecorderFake) Event(_ runtimeobj.Object, e event.Event) {
	r.events = append(r.events, string(e.Reason))
}
func (r *cmRecorderFake) WithAnnotations(_ ...string) event.Recorder { return r }
func (r *cmRecorderFake) has(reason string) bool {
	for _, e := range r.events {
		if e == reason {
			return true
		}
	}
	return false
}

func cmFactory(lk *cmLookuperFake) func(context.Context, *v1beta1.CloudManagement) (smClient.SemanticLookuper, func(), error) {
	return func(context.Context, *v1beta1.CloudManagement) (smClient.SemanticLookuper, func(), error) {
		return lk, func() {}, nil
	}
}

func cmForAdoption(name, planID string) *v1beta1.CloudManagement {
	cr := NewCloudManagement(name)
	cr.SetCreationTimestamp(metav1.NewTime(crCreatedAtCM))
	// Simulate the state left behind by a completed Create() attempt whose
	// external-name write never landed: the runtime stamped external-create-pending
	// before it called Create.
	meta.SetExternalCreatePending(cr, createPendingAtCM)
	meta.SetExternalName(cr, name) // fallback external-name == metadata.name
	cr.Status.AtProvider.DataSourceLookup = &v1beta1.CloudManagementDataSourceLookup{
		CloudManagementPlanID: planID,
	}
	return cr
}

// cmForAdoptionNoPending mirrors cmForAdoption but with NO external-create-pending
// annotation — we never attempted Create() for this CR. The heal must refuse
// to adopt anything.
func cmForAdoptionNoPending(name, planID string) *v1beta1.CloudManagement {
	cr := cmForAdoption(name, planID)
	delete(cr.GetAnnotations(), "crossplane.io/external-create-pending")
	return cr
}

func TestObserve_CloudManagementAdoption(t *testing.T) {
	notExisting := func() (cmclient.ResourcesStatus, error) {
		return cmclient.ResourcesStatus{ExternalObservation: managed.ExternalObservation{ResourceExists: false}}, nil
	}

	t.Run("match adopts compound external-name and requeues", func(t *testing.T) {
		cr := cmForAdoption("cis-1", "plan-1")
		lk := &cmLookuperFake{siID: "si-1", sbID: "sb-1", siCreatedAt: createPendingAtCM.Add(2 * time.Second), found: true}
		e := external{
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			tfClient:           TfClientFake{observeFn: notExisting},
			newAdminLookuperFn: cmFactory(lk),
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
	})

	t.Run("instance without binding yields sID-only external-name", func(t *testing.T) {
		cr := cmForAdoption("cis-2", "plan-2")
		lk := &cmLookuperFake{siID: "si-2", sbID: "", siCreatedAt: createPendingAtCM.Add(2 * time.Second), found: true}
		e := external{
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			tfClient:           TfClientFake{observeFn: notExisting},
			newAdminLookuperFn: cmFactory(lk),
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
		cr := cmForAdoption("cis-3", "plan-3")
		lk := &cmLookuperFake{found: false}
		e := external{
			kube:               &test.MockClient{MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil)},
			tfClient:           TfClientFake{observeFn: notExisting},
			newAdminLookuperFn: cmFactory(lk),
		}
		_, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if got := meta.GetExternalName(cr); got != "cis-3" {
			t.Errorf("external-name must be unchanged, got %q", got)
		}
	})

	// Regression: single-UUID external-name is NOT a fallback, so adoption
	// must NOT fire even though the compound scheme would benefit from it.
	// See internal/recovery/recovery.go for background.
	t.Run("single-UUID external-name does NOT trigger adoption (phase-1 output)", func(t *testing.T) {
		cr := cmForAdoption("cis-5", "plan-5")
		meta.SetExternalName(cr, "80540c06-2955-4bce-9c43-ad78fecc7f62") // real UUID, non-compound
		lk := &cmLookuperFake{siID: "should-not-be-set", sbID: "should-not-be-set", found: true}
		e := external{
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			tfClient:           TfClientFake{observeFn: notExisting},
			newAdminLookuperFn: cmFactory(lk),
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

	// Regression: ownership check refuses to adopt a BTP instance whose
	// created_at falls outside the window around our recorded Create attempt
	// (brownfield). Adoption is silently declined, an
	// AdoptionRefusedBrownfield Warning is emitted, and external-name stays
	// unchanged so the user can decide whether to import explicitly.
	t.Run("brownfield (BTP created outside pending window): refuses adoption, emits Warning", func(t *testing.T) {
		cr := cmForAdoption("cis-brown", "plan-brown")
		lk := &cmLookuperFake{siID: "si-brown", sbID: "sb-brown", siCreatedAt: createPendingAtCM.Add(-time.Hour), found: true}
		rec := &cmRecorderFake{}
		e := external{
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			tfClient:           TfClientFake{observeFn: notExisting},
			newAdminLookuperFn: cmFactory(lk),
			recorder:           rec,
		}
		_, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error (adoption declined), got %v", err)
		}
		if got := meta.GetExternalName(cr); got != "cis-brown" {
			t.Errorf("external-name must be unchanged, got %q", got)
		}
		if !rec.has(recovery.EventReasonRefusedBrownfield) {
			t.Errorf("expected %q event, got %+v", recovery.EventReasonRefusedBrownfield, rec.events)
		}
		if rec.has(recovery.EventReasonRecovered) {
			t.Errorf("must not record %q event when refusing brownfield", recovery.EventReasonRecovered)
		}
	})

	// New: no external-create-pending annotation means this controller never
	// attempted Create() for this CR, so the heal must short-circuit BEFORE
	// running the expensive semantic lookup. Guards the safety property that
	// motivated dropping the creationTimestamp fallback.
	t.Run("no create-pending annotation: short-circuits, does not lookup", func(t *testing.T) {
		cr := cmForAdoptionNoPending("cis-nopending", "plan-nopending")
		lk := &cmLookuperFake{siID: "must-not-be-used", sbID: "must-not-be-used", found: true}
		e := external{
			kube: &test.MockClient{
				MockUpdate:       test.NewMockUpdateFn(nil),
				MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
			},
			tfClient:           TfClientFake{observeFn: notExisting},
			newAdminLookuperFn: cmFactory(lk),
		}
		_, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if got := meta.GetExternalName(cr); got != "cis-nopending" {
			t.Errorf("external-name must be unchanged, got %q", got)
		}
		if lk.gotPlan != "" {
			t.Errorf("lookup must NOT be invoked when Create was never attempted, got planID=%q", lk.gotPlan)
		}
	})
}
