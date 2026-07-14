package subaccount

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeobj "k8s.io/apimachinery/pkg/runtime"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/adoption"
)

var crCreatedAtSA = time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

type saRecorderFake struct{ events []string }

func (r *saRecorderFake) Event(_ runtimeobj.Object, e event.Event) {
	r.events = append(r.events, string(e.Reason))
}
func (r *saRecorderFake) WithAnnotations(_ ...string) event.Recorder { return r }
func (r *saRecorderFake) has(reason string) bool {
	for _, e := range r.events {
		if e == reason {
			return true
		}
	}
	return false
}

func saWithSubdomain(subdomain string) *apisv1alpha1.Subaccount {
	cr := &apisv1alpha1.Subaccount{}
	cr.SetName(subdomain)
	cr.SetCreationTimestamp(metav1.NewTime(crCreatedAtSA))
	cr.Spec.ForProvider.Subdomain = subdomain
	// external-name intentionally left empty (fallback)
	return cr
}

func TestObserve_SubaccountAdoption(t *testing.T) {
	const guid = "b808193b-e1ee-4001-abec-f920134cca60"

	t.Run("empty external-name + subdomain match adopts and requeues", func(t *testing.T) {
		cr := saWithSubdomain("ek-test-2")
		acc := &MockAccountsApiAccessor{
			lookupGuid:      guid,
			lookupCreatedAt: crCreatedAtSA.Add(time.Hour),
			lookupFound:     true,
		}
		e := external{
			Client:           test.NewMockClient(),
			accountsAccessor: acc,
		}
		_, err := e.Observe(context.TODO(), cr)
		if !errors.Is(err, adoption.ErrRequeueAfterAdopt) {
			t.Fatalf("expected ErrRequeueAfterAdopt, got %v", err)
		}
		if meta.GetExternalName(cr) != guid {
			t.Errorf("external-name = %q, want %q", meta.GetExternalName(cr), guid)
		}
		if acc.lookupCalls != 1 {
			t.Errorf("expected exactly one subdomain lookup, got %d", acc.lookupCalls)
		}
	})

	// Regression: ownership check refuses to adopt a subaccount that predates
	// our CR (brownfield). See adoption.IsOwnedByCR.
	t.Run("brownfield (BTP created before CR): refuses adoption, emits Warning", func(t *testing.T) {
		cr := saWithSubdomain("ek-brown")
		acc := &MockAccountsApiAccessor{
			lookupGuid:      guid,
			lookupCreatedAt: crCreatedAtSA.Add(-time.Hour), // OLDER than CR
			lookupFound:     true,
		}
		rec := &saRecorderFake{}
		e := external{
			Client:           test.NewMockClient(),
			accountsAccessor: acc,
			recorder:         rec,
		}
		obs, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error (silent decline), got %v", err)
		}
		if obs.ResourceExists {
			t.Errorf("expected ResourceExists=false")
		}
		if meta.GetExternalName(cr) != "" {
			t.Errorf("external-name must stay empty (unchanged), got %q", meta.GetExternalName(cr))
		}
		if !rec.has(adoption.EventReasonRefusedBrownfield) {
			t.Errorf("expected %q event, got %+v", adoption.EventReasonRefusedBrownfield, rec.events)
		}
	})

	t.Run("no match returns ResourceExists=false and does not patch", func(t *testing.T) {
		cr := saWithSubdomain("ek-test-2")
		acc := &MockAccountsApiAccessor{lookupFound: false}
		e := external{
			Client:           test.NewMockClient(),
			accountsAccessor: acc,
		}
		obs, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if obs.ResourceExists {
			t.Errorf("expected ResourceExists=false")
		}
		if meta.GetExternalName(cr) != "" {
			t.Errorf("external-name must stay empty, got %q", meta.GetExternalName(cr))
		}
	})

	t.Run("lookup error falls through to ResourceExists=false", func(t *testing.T) {
		cr := saWithSubdomain("ek-test-2")
		acc := &MockAccountsApiAccessor{lookupErr: errors.New("boom")}
		e := external{
			Client:           test.NewMockClient(),
			accountsAccessor: acc,
		}
		obs, err := e.Observe(context.TODO(), cr)
		if err != nil {
			t.Fatalf("expected nil error (best-effort), got %v", err)
		}
		if obs.ResourceExists {
			t.Errorf("expected ResourceExists=false")
		}
	})
}
