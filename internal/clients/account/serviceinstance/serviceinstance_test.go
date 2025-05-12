package serviceinstanceclient

import (
	"context"
	"errors"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ujresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

var errTf = errors.New("tf error")

func TestConnect(t *testing.T) {
	type args struct {
		cr *v1alpha1.ServiceInstance
	}
	type fields struct {
		connector *TfConnectorMock
	}
	type want struct {
		clientReturned bool
		err            error
	}
	tests := []struct {
		name   string
		args   args
		fields fields
		want   want
	}{
		{
			name: "ConnectError",
			args: args{
				cr: &v1alpha1.ServiceInstance{},
			},
			fields: fields{
				connector: &TfConnectorMock{
					err: errTf,
				},
			},
			want: want{
				clientReturned: false,
				err:            errTf,
			},
		},
		{
			name: "SuccessfulConnect",
			args: args{
				cr: &v1alpha1.ServiceInstance{},
			},
			fields: fields{
				connector: &TfConnectorMock{
					err: nil,
				},
			},
			want: want{
				clientReturned: true,
				err:            nil,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientCreator := &ServiceInstanceClientCreator{
				connector: tc.fields.connector,
			}
			client, err := clientCreator.Connect(context.Background(), tc.args.cr)
			if err != nil && err.Error() != tc.want.err.Error() {
				t.Errorf("ServiceInstanceClientCreator.Connect() error = %v, want %v", err, tc.want.err)
			}
			if (client != nil) != tc.want.clientReturned {
				t.Errorf("ServiceInstanceClientCreator.Connect() client returned = %v, want %v", client != nil, tc.want.clientReturned)
			}

			// Verify that the Connect method was called with the correct type
			if _, ok := tc.fields.connector.CalledWithMg.(*v1alpha1.SubaccountServiceInstance); !ok {
				t.Errorf("ServiceInstanceClientCreator.Connect() called with wrong type %T, want %T", tc.fields.connector.CalledWithMg, &v1alpha1.SubaccountServiceInstance{})
			}
		})
	}
}

var _ managed.ExternalConnecter = &TfConnectorMock{}

// ClientCreator mock
type TfConnectorMock struct {
	err          error
	CalledWithMg resource.Managed // Field to store the resource passed to Connect
}

// Connect implements managed.ExternalConnecter.
func (t *TfConnectorMock) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	t.CalledWithMg = mg // Store the resource passed to Connect
	if t.err != nil {
		return nil, t.err
	}
	return &TfControllerMock{}, nil
}

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
					err: errTf,
				},
			},
			want: want{
				exists: false,
				err:    errTf,
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
			exists, err := client.Observe(context.Background())
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

func TestCreate(t *testing.T) {
	type args struct {
		cr *v1alpha1.SubaccountServiceInstance
	}
	type fields struct {
		tfClient *TfControllerMock
	}
	type want struct {
		err error
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
				cr: &v1alpha1.SubaccountServiceInstance{},
			},
			fields: fields{
				tfClient: &TfControllerMock{
					err: errTf,
				},
			},
			want: want{
				err: errTf,
			},
		},
		{
			name: "SuccessfulCreate",
			args: args{
				cr: &v1alpha1.SubaccountServiceInstance{},
			},
			fields: fields{
				tfClient: &TfControllerMock{
					err: nil,
				},
			},
			want: want{
				err: nil,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &ServiceInstanceClient{
				tfClient:          tc.fields.tfClient,
				tfServiceInstance: tc.args.cr,
			}

			err := client.Create(context.Background())
			if err != nil && err.Error() != tc.want.err.Error() {
				t.Errorf("ServiceInstanceClient.Create() error = %v, want %v", err, tc.want.err)
			}

			// Verify that the Create method was called with the correct type
			if _, ok := tc.fields.tfClient.CalledWithMg.(*v1alpha1.SubaccountServiceInstance); !ok {
				t.Errorf("ServiceInstanceClient.Create() called with wrong type %T, want %T", tc.fields.tfClient.CalledWithMg, &v1alpha1.SubaccountServiceInstance{})
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		cr *v1alpha1.SubaccountServiceInstance
	}
	type fields struct {
		tfClient *TfControllerMock
	}
	type want struct {
		err error
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
				cr: &v1alpha1.SubaccountServiceInstance{},
			},
			fields: fields{
				tfClient: &TfControllerMock{
					err: errTf,
				},
			},
			want: want{
				err: errTf,
			},
		},
		{
			name: "SuccessfulDelete",
			args: args{
				cr: &v1alpha1.SubaccountServiceInstance{},
			},
			fields: fields{
				tfClient: &TfControllerMock{
					err: nil,
				},
			},
			want: want{
				err: nil,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &ServiceInstanceClient{
				tfClient:          tc.fields.tfClient,
				tfServiceInstance: tc.args.cr,
			}

			err := client.Delete(context.Background())
			if err != nil && err.Error() != tc.want.err.Error() {
				t.Errorf("ServiceInstanceClient.Delete() error = %v, want %v", err, tc.want.err)
			}

			// Verify that the Delete method was called with the correct type
			if _, ok := tc.fields.tfClient.CalledWithMg.(*v1alpha1.SubaccountServiceInstance); !ok {
				t.Errorf("ServiceInstanceClient.Delete() called with wrong type %T, want %T", tc.fields.tfClient.CalledWithMg, &v1alpha1.SubaccountServiceInstance{})
			}
		})
	}
}

func TestQueryAsyncData(t *testing.T) {
	type args struct {
		cr *v1alpha1.SubaccountServiceInstance
	}
	type want struct {
		data *ServiceInstanceData
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"CreationInProcess": {
			reason: "No data available yet during creation",
			args: args{
				cr: tfServiceInstanceCrWithData("test-external-name", "test-id", []xpv1.Condition{
					xpv1.Available(),
					ujresource.AsyncOperationOngoingCondition(),
				}),
			},
			want: want{
				data: nil,
			},
		},
		"DataAvailable": {
			reason: "Data is available after creation",
			args: args{
				cr: tfServiceInstanceCrWithData("test-external-name", "test-id", []xpv1.Condition{
					xpv1.Available(),
					ujresource.AsyncOperationFinishedCondition(),
				}),
			},
			want: want{
				data: &ServiceInstanceData{
					Conditions: []xpv1.Condition{
						xpv1.Available(),
						ujresource.AsyncOperationFinishedCondition(),
					},
					ExternalName: "test-external-name",
					ID:           "test-id",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := &ServiceInstanceClient{
				tfServiceInstance: tc.args.cr,
			}
			data := client.QueryAsyncData(context.Background())

			if diff := cmp.Diff(tc.want.data, data); diff != "" {
				t.Errorf("\nServiceInstanceClient.QueryAsyncData(...): -want, +got:\n%s\n", diff)
			}

		})
	}
}

// tfServiceInstanceCrWithData is a helper function to create a SubaccountServiceInstance resource with the given data
func tfServiceInstanceCrWithData(externalName, id string, conditions []xpv1.Condition) *v1alpha1.SubaccountServiceInstance {
	cr := &v1alpha1.SubaccountServiceInstance{
		Status: v1alpha1.SubaccountServiceInstanceStatus{
			ResourceStatus: xpv1.ResourceStatus{
				ConditionedStatus: xpv1.ConditionedStatus{
					Conditions: conditions,
				},
			},
			AtProvider: v1alpha1.SubaccountServiceInstanceObservation{
				ID: &id,
			},
		},
	}
	meta.SetExternalName(cr, externalName)
	return cr
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
	t.CalledWithMg = mg
	if t.err != nil {
		return managed.ExternalCreation{}, t.err
	}
	return managed.ExternalCreation{}, nil
}

// Delete implements managed.ExternalClient.
func (t *TfControllerMock) Delete(ctx context.Context, mg resource.Managed) error {
	t.CalledWithMg = mg
	if t.err != nil {
		return t.err
	}
	return nil
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
