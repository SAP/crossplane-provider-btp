package servicebinding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	servicebindingclient "github.com/sap/crossplane-provider-btp/internal/clients/account/servicebinding"
)

// sbRotating builds a rotation-enabled ServiceBinding CR with a resolved parent
// instance and a recorded create-pending annotation (so the ownership check has
// a reference point). The external-name is left as the CR-name fallback.
func sbRotating(name, siID, bindingName string) *v1alpha1.ServiceBinding {
	cr := sbForAdoption(name, siID, bindingName)
	cr.Spec.Rotation = &v1alpha1.RotationParameters{
		Frequency: &providerv1alpha1.Duration{Duration: time.Hour},
		TTL:       &providerv1alpha1.Duration{Duration: 2 * time.Hour},
	}
	return cr
}

// TestCreate_Idempotency covers Option A: the create name is committed to the
// PendingBindingNameKey annotation before Service Manager is called, reused on
// retries, and a binding a prior attempt already created is adopted instead of
// duplicated.
func TestCreate_Idempotency(t *testing.T) {
	const createdGUID = "12345678-1234-5678-9abc-123456789012" // MockServiceBindingClient.Create
	const adoptGUID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	t.Run("commits pending name before SM create and clears it after", func(t *testing.T) {
		cr := sbRotating("sb-1", "si-1", "test-binding")
		var updates int
		factory := &MockServiceBindingClientFactory{Client: &MockServiceBindingClient{}}
		e := external{
			kube: &test.MockClient{MockUpdate: func(_ context.Context, _ kubeclient.Object, _ ...kubeclient.UpdateOption) error {
				updates++
				return nil
			}},
			clientFactory: factory,
			// no lookuper: with newAdminLookuperFn nil, lookupOwnedBinding is a
			// no-op returning (‑, false, nil) so a create proceeds.
			nameGenerator: func(base string) string { return base + "-fixed1" },
		}

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// external-name is the freshly created GUID; both markers are cleared.
		if got := meta.GetExternalName(cr); got != createdGUID {
			t.Errorf("external-name = %q, want %q", got, createdGUID)
		}
		if _, ok := cr.GetAnnotations()[servicebindingclient.PendingBindingNameKey]; ok {
			t.Errorf("pending-name annotation must be cleared after successful create")
		}
		// One SM create with the committed name.
		if len(factory.CreateClientCalls) != 1 {
			t.Fatalf("want 1 CreateClient call, got %d", len(factory.CreateClientCalls))
		}
		if factory.CreateClientCalls[0].TargetName != "test-binding-fixed1" {
			t.Errorf("SM create name = %q, want test-binding-fixed1", factory.CreateClientCalls[0].TargetName)
		}
		// Two persists: one committing the pending name, one recording external-name.
		if updates != 2 {
			t.Errorf("want 2 kube.Update calls (commit + record), got %d", updates)
		}
	})

	t.Run("reuses committed pending name on retry instead of regenerating", func(t *testing.T) {
		cr := sbRotating("sb-2", "si-2", "test-binding")
		// Simulate a prior attempt that committed the name but never recorded a result.
		meta.AddAnnotations(cr, map[string]string{servicebindingclient.PendingBindingNameKey: "test-binding-prior"})

		factory := &MockServiceBindingClientFactory{Client: &MockServiceBindingClient{}}
		e := external{
			kube:          &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			clientFactory: factory,
			// A different generator suffix would be a bug if used; assert it is NOT.
			nameGenerator: func(base string) string { return base + "-SHOULD-NOT-USE" },
		}

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(factory.CreateClientCalls) != 1 {
			t.Fatalf("want 1 CreateClient call, got %d", len(factory.CreateClientCalls))
		}
		if got := factory.CreateClientCalls[0].TargetName; got != "test-binding-prior" {
			t.Errorf("SM create name = %q, want the committed test-binding-prior", got)
		}
	})

	t.Run("adopts an owned binding created by a prior lost attempt, skips SM create", func(t *testing.T) {
		cr := sbRotating("sb-3", "si-3", "test-binding")
		meta.AddAnnotations(cr, map[string]string{servicebindingclient.PendingBindingNameKey: "test-binding-prior"})

		lk := &sbLookuperFake{guid: adoptGUID, createdAt: createPendingAtSB.Add(2 * time.Second), found: true}
		factory := &MockServiceBindingClientFactory{Client: &MockServiceBindingClient{}}
		e := external{
			kube:               &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			clientFactory:      factory,
			newAdminLookuperFn: sbFactory(lk),
			nameGenerator:      func(base string) string { return base + "-fixed1" },
		}

		creation, err := e.Create(context.Background(), cr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(creation.ConnectionDetails) != 0 {
			t.Errorf("adoption must not return connection details")
		}
		if got := meta.GetExternalName(cr); got != adoptGUID {
			t.Errorf("external-name = %q, want adopted %q", got, adoptGUID)
		}
		if len(factory.CreateClientCalls) != 0 {
			t.Errorf("must NOT create when adopting, got %d CreateClient calls", len(factory.CreateClientCalls))
		}
		if lk.gotName != "test-binding-prior" {
			t.Errorf("lookup name = %q, want committed test-binding-prior", lk.gotName)
		}
		if _, ok := cr.GetAnnotations()[servicebindingclient.PendingBindingNameKey]; ok {
			t.Errorf("pending-name annotation must be cleared after adoption")
		}
	})

	t.Run("lookup error aborts before creating (no duplicate)", func(t *testing.T) {
		cr := sbRotating("sb-4", "si-4", "test-binding")
		meta.AddAnnotations(cr, map[string]string{servicebindingclient.PendingBindingNameKey: "test-binding-prior"})

		lk := &sbLookuperFake{err: errors.New("SM unavailable")}
		factory := &MockServiceBindingClientFactory{Client: &MockServiceBindingClient{}}
		e := external{
			kube:               &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			clientFactory:      factory,
			newAdminLookuperFn: sbFactory(lk),
			nameGenerator:      func(base string) string { return base + "-fixed1" },
		}

		if _, err := e.Create(context.Background(), cr); err == nil {
			t.Fatalf("expected error when lookup fails, got nil")
		}
		if len(factory.CreateClientCalls) != 0 {
			t.Errorf("must NOT create when lookup errors, got %d CreateClient calls", len(factory.CreateClientCalls))
		}
	})

	t.Run("committed-name match is adopted even outside the IsOwnedByCR window", func(t *testing.T) {
		cr := sbRotating("sb-5", "si-5", "test-binding")
		meta.AddAnnotations(cr, map[string]string{servicebindingclient.PendingBindingNameKey: "test-binding-prior"})

		// createdAt far outside the ownership window: on the heal path this would
		// be refused as brownfield, but the committed pending name carries a
		// random suffix we generated and persisted, so a match under it is
		// necessarily our own prior attempt. Adopt it rather than duplicate-create
		// (crossplane-runtime refreshes external-create-pending before each retry,
		// which would otherwise push the prior binding below the window).
		lk := &sbLookuperFake{guid: adoptGUID, createdAt: createPendingAtSB.Add(-time.Hour), found: true}
		factory := &MockServiceBindingClientFactory{Client: &MockServiceBindingClient{}}
		e := external{
			kube:               &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			clientFactory:      factory,
			newAdminLookuperFn: sbFactory(lk),
			nameGenerator:      func(base string) string { return base + "-fixed1" },
		}

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Adopted: no create ran, external-name is the adopted GUID.
		if len(factory.CreateClientCalls) != 0 {
			t.Fatalf("must NOT create when adopting, got %d CreateClient calls", len(factory.CreateClientCalls))
		}
		if got := meta.GetExternalName(cr); got != adoptGUID {
			t.Errorf("external-name = %q, want adopted %q", got, adoptGUID)
		}
	})

	t.Run("non-rotated binding uses stable spec name, no pending annotation", func(t *testing.T) {
		cr := &v1alpha1.ServiceBinding{}
		cr.SetName("sb-6")
		cr.Spec.ForProvider.Name = "plain-binding"
		cr.Spec.ForProvider.ServiceInstanceID = internal.Ptr("si-6")

		factory := &MockServiceBindingClientFactory{Client: &MockServiceBindingClient{}}
		e := external{
			kube:          &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			clientFactory: factory,
			nameGenerator: func(base string) string { return base + "-SHOULD-NOT-USE" },
		}

		if _, err := e.Create(context.Background(), cr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(factory.CreateClientCalls) != 1 {
			t.Fatalf("want 1 CreateClient call, got %d", len(factory.CreateClientCalls))
		}
		if got := factory.CreateClientCalls[0].TargetName; got != "plain-binding" {
			t.Errorf("SM create name = %q, want stable plain-binding", got)
		}
		if _, ok := cr.GetAnnotations()[servicebindingclient.PendingBindingNameKey]; ok {
			t.Errorf("non-rotated create must not set a pending-name annotation")
		}
	})
}
