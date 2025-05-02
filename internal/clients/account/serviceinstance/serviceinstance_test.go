package serviceinstanceclient

import (
	"context"
	"errors"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

var tfError = errors.New("tf error")

func TestObserve(t *testing.T) {

	type fields struct {
		tfClient managed.ExternalClient
	}

	type args struct {
		cr *v1alpha1.ServiceInstance
	}

	type want struct {
		exists bool
		err    error
	}

	tests := []struct {
		name   string
		reason string
		args   args
		fields fields
		want   want
	}{
		{
			name:   "TfError",
			reason: "tf error should be returned from client",
			args: args{
				cr: &v1alpha1.ServiceInstance{},
			},
			fields: fields{
				tfClient: &TfControllerMock{
					err: tfError,
				},
			},
			want: want{
				exists: false,
				err:    tfError,
			},
		},
		{
			name:   "ResourceExists",
			reason: "resourceExists should be returned as true when tfClient returns an observation with ResourceExists set to true",
			args: args{
				cr: &v1alpha1.ServiceInstance{},
			},
			fields: fields{
				tfClient: &TfControllerMock{
					err: nil,
					observation: managed.ExternalObservation{
						ResourceExists: true,
					},
				},
			},
			want: want{
				exists: true,
				err:    nil,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &ServiceInstanceClient{tfClient: tc.fields.tfClient}
			exists, err := client.Observe(context.Background(), tc.args.cr)
			if err != tc.want.err {
				t.Fatalf("\n%s\n, ServiceInstanceClient.Observe() error = %v, want %v", tc.reason, err, tc.want.err)

			}
			if exists != tc.want.exists {
				t.Fatalf("\n%s\n, ServiceInstanceClient.Observe() exists = %v, want %v", tc.reason, exists, tc.want.exists)
			}
		})
	}

}

var _ managed.ExternalClient = &TfControllerMock{}

type TfControllerMock struct {
	err         error
	observation managed.ExternalObservation
}

// Create implements managed.ExternalClient.
func (t *TfControllerMock) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	panic("unimplemented")
}

// Delete implements managed.ExternalClient.
func (t *TfControllerMock) Delete(ctx context.Context, mg resource.Managed) error {
	panic("unimplemented")
}

// Observe implements managed.ExternalClient.
func (t *TfControllerMock) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	return t.observation, t.err
}

// Update implements managed.ExternalClient.
func (t *TfControllerMock) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	panic("unimplemented")
}
