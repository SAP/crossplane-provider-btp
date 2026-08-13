package servicemanager

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	sm "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	obsInstanceUUID = "6aa64c2f-38c1-49a9-b2e8-cf9fea769b7f"
	obsBindingUUID  = "9c2b1f80-3d4e-4a11-8f2c-7b5d6e1a4c33"
)

// ADR(external-name) Observe() step 2: a malformed key must error rather than be
// silently deconstructed. TfClientFake keeps nil function fields, so reaching BTP
// would panic; that is the assertion that validation runs first.
func TestObserve_RejectsMalformedExternalName(t *testing.T) {
	const crName = "my-service-manager"

	for name, externalName := range map[string]string{
		"three segments":     obsInstanceUUID + "/" + obsBindingUUID + "/" + obsInstanceUUID,
		"non-uuid segment":   obsInstanceUUID + "/nope",
		"trailing separator": obsInstanceUUID + "/",
	} {
		t.Run(name, func(t *testing.T) {
			cr := NewServiceManager(crName, WithExternalName(externalName))

			_, err := (&external{tfClient: &TfClientFake{}}).Observe(context.Background(), cr)
			if err == nil {
				t.Fatalf("Observe() with external-name %q = nil error, want a format error", externalName)
			}
			if !errors.Is(err, sm.ErrInvalidExternalName) {
				t.Fatalf("Observe() with external-name %q = %v, want errors.Is(_, ErrInvalidExternalName)", externalName, err)
			}
			// The sentinel alone is satisfied by returning ValidateExternalName's
			// error bare. Pin the remediation text too, since that is what reaches
			// the operator through the Synced condition and the event.
			if !strings.Contains(err.Error(), errExternalNameFormat) {
				t.Fatalf("Observe() with external-name %q = %v, want the message to contain %q", externalName, err, errExternalNameFormat)
			}
		})
	}
}

// The fallback and two-phase forms must survive validation. Rejecting them would
// brick the recovery path and phase 2 of create respectively.
func TestObserve_AcceptsFallbackAndTwoPhaseExternalNames(t *testing.T) {
	const crName = "my-service-manager"

	for name, externalName := range map[string]string{
		"unset":                  "",
		"metadata.name fallback": crName,
		"phase-1 instance only":  obsInstanceUUID,
		"steady-state compound":  obsInstanceUUID + "/" + obsBindingUUID,
	} {
		t.Run(name, func(t *testing.T) {
			cr := NewServiceManager(crName, WithExternalName(externalName))

			observed := false
			client := &TfClientFake{observeFn: func() (sm.ResourcesStatus, error) {
				observed = true
				return sm.ResourcesStatus{}, nil
			}}
			// setStatus runs after ObserveResources and writes through kube, so a
			// status-update mock is required to reach the return without panicking.
			e := external{
				tfClient: client,
				kube:     &test.MockClient{MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil)},
			}
			_, err := e.Observe(context.Background(), cr)
			if err != nil {
				t.Fatalf("Observe() with external-name %q = %v, want nil", externalName, err)
			}
			if !observed {
				t.Fatalf("Observe() with external-name %q never reached ObserveResources", externalName)
			}
		})
	}
}

// A malformed key must not block deletion. An Observe() error returns before the
// reconciler's WasDeleted branch, so validating here would strand the finalizer
// and still leave the BTP resources behind. Mirrors the guard in
// config/btp_subaccount_api_credential/config.go.
func TestObserve_DoesNotValidateWhileDeleting(t *testing.T) {
	const crName = "my-service-manager"

	cr := NewServiceManager(crName,
		WithExternalName(obsInstanceUUID+"/"+obsBindingUUID+"/"+obsInstanceUUID),
		WithDeletionTimestamp(metav1.Now()),
	)

	observed := false
	e := external{
		tfClient: &TfClientFake{observeFn: func() (sm.ResourcesStatus, error) {
			observed = true
			return sm.ResourcesStatus{}, nil
		}},
		kube: &test.MockClient{MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil)},
	}

	if _, err := e.Observe(context.Background(), cr); err != nil {
		t.Fatalf("Observe() on a deleting CR with a malformed external-name = %v, want nil", err)
	}
	if !observed {
		t.Fatal("Observe() on a deleting CR never reached ObserveResources: validation blocked the delete path")
	}
}
