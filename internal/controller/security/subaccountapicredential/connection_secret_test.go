package subaccountapicredential

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

func TestConnectionSecretValidatingExternalObserve(t *testing.T) {
	complete := map[string][]byte{
		"attribute.api_url":       []byte("api-url"),
		"attribute.client_id":     []byte("client-id"),
		"attribute.client_secret": []byte("client-secret"),
		"attribute.token_url":     []byte("token-url"),
	}
	newResource := func() *v1alpha1.SubaccountApiCredential {
		return &v1alpha1.SubaccountApiCredential{
			Spec: v1alpha1.SubaccountApiCredentialSpec{
				ResourceSpec: xpv1.ResourceSpec{WriteConnectionSecretToReference: &xpv1.SecretReference{Name: "connection", Namespace: "workloads"}},
			},
		}
	}

	cases := map[string]struct {
		observation managed.ExternalObservation
		data        map[string][]byte
		resource    func() *v1alpha1.SubaccountApiCredential
		getErr      error
		observeErr  error
		wantErr     string
	}{
		"existing external with malformed Secret": {
			observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
			data:        map[string][]byte{"attribute.client_id": []byte("client-id")},
			wantErr:     `connection Secret "workloads/connection" is incomplete: missing required field(s): attribute.api_url, attribute.client_secret, attribute.token_url`,
		},
		"existing external with complete Secret": {
			observation: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
			data:        complete,
		},
		"external absent skips validation and permits Create": {
			observation: managed.ExternalObservation{ResourceExists: false},
			data:        map[string][]byte{},
		},
		"deletion skips validation": {
			observation: managed.ExternalObservation{ResourceExists: true},
			data:        map[string][]byte{},
			resource: func() *v1alpha1.SubaccountApiCredential {
				cr := newResource()
				now := metav1.Now()
				cr.DeletionTimestamp = &now
				return cr
			},
		},
		"delegate observe error is returned unchanged": {
			observation: managed.ExternalObservation{},
			observeErr:  errors.New("external observation failed"),
			getErr:      errors.New("Secret must not be read"),
			wantErr:     "external observation failed",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := newResource()
			if tc.resource != nil {
				cr = tc.resource()
			}
			kube := &test.MockClient{MockGet: test.NewMockGetFn(tc.getErr, func(obj client.Object) error {
				secret := obj.(*corev1.Secret)
				secret.Data = tc.data
				return nil
			})}
			delegate := &managed.ExternalClientFns{
				ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
					return tc.observation, tc.observeErr
				},
			}
			external := &connectionSecretValidatingExternal{delegate: delegate, kube: kube}

			got, err := external.Observe(context.Background(), cr)
			if diff := cmp.Diff(tc.observation, got); diff != "" {
				t.Errorf("observation mismatch (-want +got):\n%s", diff)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Observe() unexpected error: %v", err)
				}
			} else if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("Observe() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestConnectionSecretValidatingExternalDelegatesCreate(t *testing.T) {
	called := false
	delegate := &managed.ExternalClientFns{
		CreateFn: func(context.Context, resource.Managed) (managed.ExternalCreation, error) {
			called = true
			return managed.ExternalCreation{}, nil
		},
	}
	external := &connectionSecretValidatingExternal{delegate: delegate}
	if _, err := external.Create(context.Background(), &v1alpha1.SubaccountApiCredential{}); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if !called {
		t.Fatal("Create() did not delegate")
	}
}

func TestConnectionSecretValidatingConnectorConnect(t *testing.T) {
	called := false
	client := &managed.ExternalClientFns{
		DisconnectFn: func(context.Context) error { return nil },
	}
	connector := managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) {
		called = true
		return client, nil
	})

	got, err := NewConnectionSecretValidatingConnector(connector, &test.MockClient{}).Connect(context.Background(), &v1alpha1.SubaccountApiCredential{})
	if err != nil {
		t.Fatalf("Connect() unexpected error: %v", err)
	}
	if !called || got == nil {
		t.Fatalf("Connect() called = %v, client = %v", called, got)
	}
}
