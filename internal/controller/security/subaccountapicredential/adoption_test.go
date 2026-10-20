package subaccountapicredential

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAdoptionRejectingConnectorFinalizesRejectedAdoptionLocally(t *testing.T) {
	now := metav1.Now()
	cr := &v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now}}
	cr.SetConditions(xpv1.Unavailable().WithMessage(errUnsupportedAdoption))
	connected := false
	connector := &adoptionRejectingConnector{ExternalConnector: managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) {
		connected = true
		return nil, nil
	})}
	client, err := connector.Connect(context.Background(), cr)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if connected {
		t.Fatal("underlying connector was called for failed-adoption cleanup")
	}
	observation, err := client.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if observation.ResourceExists {
		t.Fatal("failed adoption must be treated as locally absent during cleanup")
	}
}

func TestAdoptionRejectingClientCreateCollision(t *testing.T) {
	cr := &v1alpha1.SubaccountApiCredential{}
	inner := managed.ExternalClientFns{
		CreateFn: func(context.Context, resource.Managed) (managed.ExternalCreation, error) {
			return managed.ExternalCreation{}, errors.New("credential already exists")
		},
	}
	_, err := (&adoptionRejectingClient{ExternalClient: inner}).Create(context.Background(), cr)
	if err == nil || !strings.Contains(err.Error(), errUnsupportedAdoption) {
		t.Fatalf("expected explicit adoption diagnostic, got %v", err)
	}
	if ready := cr.GetCondition(xpv1.TypeReady); ready.Status != corev1.ConditionFalse || !strings.Contains(ready.Message, errUnsupportedAdoption) {
		t.Errorf("expected explicit unavailable condition, got %#v", ready)
	}
}

func TestAdoptionRejectingClientObserve(t *testing.T) {
	publicDetails := map[string][]byte{
		"attribute.client_id": []byte("public-id"),
		"attribute.api_url":   []byte("https://api.example.test"),
	}
	cases := map[string]struct {
		exists             bool
		alreadyOwned       bool
		previouslyRejected bool
		deleting           bool
		observeErr         error
		wantAdoption       bool
	}{
		"existing target on fresh MR": {exists: true, wantAdoption: true},
		"target absent":               {},
		"previously acquired target":  {exists: true, alreadyOwned: true},
		"repeated rejected adoption":  {exists: true, alreadyOwned: true, previouslyRejected: true, wantAdoption: true},
		"deleting MR":                 {exists: true, deleting: true},
		"read failure":                {exists: true, observeErr: context.DeadlineExceeded},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := &v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: "credential"}}
			meta.SetExternalName(cr, "subaccount-guid/credential-name")
			if tc.alreadyOwned {
				cr.Status.AtProvider.ID = internal.Ptr("provider-resource-id")
			}
			if tc.previouslyRejected {
				cr.SetConditions(xpv1.Unavailable().WithMessage(errUnsupportedAdoption))
			}
			if tc.deleting {
				cr.DeletionTimestamp = internal.Ptr(metav1.Now())
			}

			inner := managed.ExternalClientFns{
				ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
					return managed.ExternalObservation{ResourceExists: tc.exists, ResourceUpToDate: true, ConnectionDetails: publicDetails}, tc.observeErr
				},
				CreateFn: func(context.Context, resource.Managed) (managed.ExternalCreation, error) {
					return managed.ExternalCreation{}, nil
				},
				UpdateFn: func(context.Context, resource.Managed) (managed.ExternalUpdate, error) {
					return managed.ExternalUpdate{}, nil
				},
				DeleteFn: func(context.Context, resource.Managed) (managed.ExternalDelete, error) {
					return managed.ExternalDelete{}, nil
				},
				DisconnectFn: func(context.Context) error { return nil },
			}
			got, err := (&adoptionRejectingClient{ExternalClient: inner}).Observe(context.Background(), cr)
			if tc.wantAdoption {
				if err == nil || !strings.Contains(err.Error(), errUnsupportedAdoption) {
					t.Fatalf("expected explicit adoption error, got %v", err)
				}
				if !got.ResourceExists || !got.ResourceUpToDate || string(got.ConnectionDetails["attribute.client_id"]) != "public-id" {
					t.Errorf("partial public observation should be retained: %#v", got)
				}
				if ready := cr.GetCondition(xpv1.TypeReady); ready.Status != corev1.ConditionFalse || !strings.Contains(ready.Message, errUnsupportedAdoption) {
					t.Errorf("expected explicit unavailable condition, got %#v", ready)
				}
				return
			}
			if err != tc.observeErr {
				t.Fatalf("expected Observe error %v, got %v", tc.observeErr, err)
			}
		})
	}
}
