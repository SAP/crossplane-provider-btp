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
		tfClient *TfControllerMock
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
		args   args
		fields fields
		want   want
	}{
		{
			name: "TfError",
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
			name: "ResourceExists",
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
				t.Errorf("ServiceInstanceClient.Observe() error = %v, want %v", err, tc.want.err)

			}
			if exists != tc.want.exists {
				t.Errorf("ServiceInstanceClient.Observe() exists = %v, want %v", exists, tc.want.exists)
			}

			//verify CalledWithMg is of type SubaccountServiceInstance
			if _, ok := tc.fields.tfClient.CalledWithMg.(*v1alpha1.SubaccountServiceInstance); !ok {
				t.Errorf("ServiceInstanceClient.Observe() called with wrong type %T, want %T", tc.fields.tfClient.CalledWithMg, &v1alpha1.ServiceInstance{})
			}

		})
	}

}

var _ managed.ExternalClient = &TfControllerMock{}

type TfControllerMock struct {
	err         error
	observation managed.ExternalObservation

	// CalledWithMg is used to check if the correct resource was passed to the methods
	CalledWithMg resource.Managed
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
	t.CalledWithMg = mg
	return t.observation, t.err
}

// Update implements managed.ExternalClient.
func (t *TfControllerMock) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	panic("unimplemented")
}
