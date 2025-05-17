package tfclient

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ujresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/crossplane/upjet/pkg/resource/fake"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var errTf = errors.New("tf error")

func TestConnect(t *testing.T) {
	type args struct {
		cr ManagedMock
	}
	type fields struct {
		connector *TfConnectorMock
		tfMapper  TfMapper[ManagedMock, *fake.Terraformed]
		kube      client.Client
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
				cr: ManagedMock{},
			},
			fields: fields{
				connector: &TfConnectorMock{
					err: errTf,
				},
				tfMapper: &TfMapperMock{},
				kube:     nil,
			},
			want: want{
				clientReturned: false,
				err:            errTf,
			},
		},
		{
			name: "SuccessfulConnect",
			args: args{
				cr: ManagedMock{},
			},
			fields: fields{
				connector: &TfConnectorMock{
					err: nil,
				},
				tfMapper: &TfMapperMock{},
				kube:     nil,
			},
			want: want{
				clientReturned: true,
				err:            nil,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			tfConnector := &TfProxyConnector[ManagedMock, *fake.Terraformed]{
				tfMapper:  tc.fields.tfMapper,
				connector: tc.fields.connector,
				kube:      tc.fields.kube,
			}

			client, err := tfConnector.Connect(context.Background(), tc.args.cr)
			if err != nil && err.Error() != tc.want.err.Error() {
				t.Errorf("TfProxyConnector.Connect() error = %v, want %v", err, tc.want.err)
			}
			if (client != nil) != tc.want.clientReturned {
				t.Errorf("TfProxyConnector.Connect() client returned = %v, want %v", client != nil, tc.want.clientReturned)
			}

			// Verify that the Connect method was called with the correct type
			if _, ok := tc.fields.connector.CalledWithMg.(*fake.Terraformed); !ok {
				t.Errorf("TfProxyConnector.Connect() called with wrong type %T, want %T", tc.fields.connector.CalledWithMg, &fake.Terraformed{})
			}
		})
	}
}

func TestObserve(t *testing.T) {

	type fields struct {
		tfClient *TfControllerMock
	}

	type want struct {
		exists bool
		err    error
	}

	tests := []struct {
		name   string
		fields fields
		want   want
	}{
		{
			name: "TfError",
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
			controller := &TfProxyController[*fake.Terraformed]{
				tfClient: tc.fields.tfClient,
			}

			exists, err := controller.Observe(context.Background())
			if err != tc.want.err {
				t.Errorf("TfProxyController.Observe() error = %v, want %v", err, tc.want.err)

			}
			if exists != tc.want.exists {
				t.Errorf("TfProxyController.Observe() exists = %v, want %v", exists, tc.want.exists)
			}

			//verify CalledWithMg is of type
			if _, ok := tc.fields.tfClient.CalledWithMg.(*fake.Terraformed); !ok {
				t.Errorf("TfProxyController.Observe() called with wrong type %T, want %T", tc.fields.tfClient.CalledWithMg, &fake.Terraformed{})
			}

		})
	}
}

func TestCreate(t *testing.T) {
	type fields struct {
		tfClient *TfControllerMock
	}
	type want struct {
		err error
	}
	tests := []struct {
		name   string
		fields fields
		want   want
	}{
		{
			name: "TfError",
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
			client := &TfProxyController[*fake.Terraformed]{
				tfClient: tc.fields.tfClient,
			}

			err := client.Create(context.Background())
			if err != nil && err.Error() != tc.want.err.Error() {
				t.Errorf("TfProxyController.Create() error = %v, want %v", err, tc.want.err)
			}

			// Verify that the Create method was called with the correct type
			if _, ok := tc.fields.tfClient.CalledWithMg.(*fake.Terraformed); !ok {
				t.Errorf("TfProxyController.Create() called with wrong type %T, want %T", tc.fields.tfClient.CalledWithMg, &fake.Terraformed{})
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type fields struct {
		tfClient *TfControllerMock
	}
	type want struct {
		err error
	}
	tests := []struct {
		name   string
		fields fields
		want   want
	}{
		{
			name: "TfError",
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
			client := &TfProxyController[*fake.Terraformed]{
				tfClient: tc.fields.tfClient,
			}

			err := client.Delete(context.Background())
			if err != nil && err.Error() != tc.want.err.Error() {
				t.Errorf("TfProxyController.Delete() error = %v, want %v", err, tc.want.err)
			}

			// Verify that the Delete method was called with the correct type
			if _, ok := tc.fields.tfClient.CalledWithMg.(*fake.Terraformed); !ok {
				t.Errorf("TfProxyController.Delete() called with wrong type %T, want %T", tc.fields.tfClient.CalledWithMg, &fake.Terraformed{})
			}
		})
	}
}

func TestQueryAsyncData(t *testing.T) {
	type args struct {
		cr *fake.Terraformed
	}
	type want struct {
		data *ObservationData
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"CreationInProcess": {
			reason: "No data available yet during creation",
			args: args{
				cr: terraformedCrWithData("test-external-name", "test-id", []xpv1.Condition{
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
				cr: terraformedCrWithData("test-external-name", "test-id", []xpv1.Condition{
					xpv1.Available(),
					ujresource.AsyncOperationFinishedCondition(),
				}),
			},
			want: want{
				data: &ObservationData{
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
			client := &TfProxyController[*fake.Terraformed]{
				tfResource: tc.args.cr,
			}
			data := client.QueryAsyncData(context.Background())

			if diff := cmp.Diff(tc.want.data, data); diff != "" {
				t.Errorf("\nTfProxyController.QueryAsyncData(...): -want, +got:\n%s\n", diff)
			}

		})
	}
}

// acts as dummy for generic testing
type ManagedMock struct {
	resource.Managed
}

var _ managed.ExternalConnecter = &TfConnectorMock{}

// TF external connector mock
type TfConnectorMock struct {
	err          error
	CalledWithMg resource.Managed // Field to store the resource passed to Connect
}

func (t *TfConnectorMock) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	t.CalledWithMg = mg // Store the resource passed to Connect
	if t.err != nil {
		return nil, t.err
	}
	return &TfControllerMock{}, nil
}

var _ TfMapper[ManagedMock, *fake.Terraformed] = &TfMapperMock{}

type TfMapperMock struct {
}

// TfResource implements TfMapper.
func (t *TfMapperMock) TfResource(cr ManagedMock, kube client.Client) *fake.Terraformed {
	return &fake.Terraformed{}
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

// terraformedCrWithData is a helper function to create a terraformed resource with the given data
func terraformedCrWithData(externalName, id string, conditions []xpv1.Condition) *fake.Terraformed {
	cr := &fake.Terraformed{}
	cr.ID = id

	for _, condition := range conditions {
		cr.SetConditions(condition)
	}
	meta.SetExternalName(cr, externalName)
	return cr
}
