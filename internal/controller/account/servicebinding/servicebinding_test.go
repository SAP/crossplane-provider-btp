package servicebinding

import (
	"context"
	"errors"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	tfClient "github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	ujresource "github.com/crossplane/upjet/pkg/resource"
)

var (
	errClient  = errors.New("apiError")
	errKube    = errors.New("kubeError")
	errCreator = errors.New("creatorError")
)

func TestObserve(t *testing.T) {
	type fields struct {
		client *TfProxyMock
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		o   managed.ExternalObservation
		err error
		cr  *v1alpha1.ServiceBinding // Expected complete CR
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"LookupError": {
			reason: "error should be returned",
			fields: fields{
				client: &TfProxyMock{err: errClient},
			},
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err: errClient,
				cr:  buildExpectedServiceBinding(), // No annotations, observation data, or conditions
			},
		},
		"NotFound": {
			reason: "should return not existing",
			fields: fields{
				client: &TfProxyMock{status: tfClient.NotExisting},
			},
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				cr: buildExpectedServiceBinding(), // No annotations, observation data, or conditions
			},
		},
		"Requires Update": {
			reason: "should return up to date == false",
			fields: fields{
				client: &TfProxyMock{status: tfClient.Drift},
			},
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: buildExpectedServiceBinding(), // No annotations, observation data, or conditions
			},
		},
		"Happy, while async in process": {
			reason: "should return existing, but no data, or connection details",
			fields: fields{
				client: &TfProxyMock{
					status:  tfClient.UpToDate,
					details: map[string][]byte{},
				},
			},
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: buildExpectedServiceBinding(), // No annotations, observation data, or conditions
			},
		},
		"Happy, no drift": {
			reason: "should return existing and pull data from embedded tf resource",
			fields: fields{
				client: &TfProxyMock{
					status: tfClient.UpToDate,
					data: &tfClient.ObservationData{
						ExternalName: "some-ext-name",
						ID:           "some-id",
					},
					details: map[string][]byte{
						"some-key": []byte("some-value"),
					},
				},
			},
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
					ConnectionDetails: managed.ConnectionDetails{
						"some-key": []byte("some-value"),
					},
				},
				cr: buildExpectedServiceBinding(
					withExternalName("some-ext-name"),
					withObservationData("some-id"),
					withConditions(xpv1.Available()),
				),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				tfClient: tc.fields.client,
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
			}

			got, err := e.Observe(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}

			// Verify the entire CR
			cr, ok := tc.args.mg.(*v1alpha1.ServiceBinding)
			if !ok {
				t.Fatalf("expected *v1alpha1.ServiceBinding, got %T", tc.args.mg)
			}
			if diff := cmp.Diff(tc.want.cr, cr); diff != "" {
				t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type fields struct {
		client *TfProxyMock
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		err error
		cr  *v1alpha1.ServiceBinding // Expected complete CR after creation
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"ApiError": {
			reason: "should return an error when the API call fails",
			fields: fields{
				client: &TfProxyMock{err: errClient},
			},
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err: errClient,
				cr: buildExpectedServiceBinding(
					withConditions(
						xpv1.Creating(),
					),
				),
			},
		},
		"HappyPath": {
			reason: "should create the resource successfully and set Creating condition",
			fields: fields{
				client: &TfProxyMock{},
			},
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err: nil,
				cr: buildExpectedServiceBinding(
					withConditions(
						xpv1.Creating(),
					),
				),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				tfClient: tc.fields.client,
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
			}

			_, err := e.Create(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)

			// Verify the entire CR
			cr, ok := tc.args.mg.(*v1alpha1.ServiceBinding)
			if !ok {
				t.Fatalf("expected *v1alpha1.ServiceBinding, got %T", tc.args.mg)
			}
			if diff := cmp.Diff(tc.want.cr, cr); diff != "" {
				t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestConnect(t *testing.T) {
	type fields struct {
		creator *TfProxyClientCreatorMock
	}

	type args struct {
		mg resource.Managed
	}

	type want struct {
		err            error
		externalExists bool
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"ConnectError": {
			reason: "should return an error when the creator fails",
			fields: fields{
				creator: &TfProxyClientCreatorMock{err: errCreator},
			},
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err: errCreator,
			},
		},
		"ConnectSuccess": {
			reason: "should return a client when the creator succeeds",
			fields: fields{
				creator: &TfProxyClientCreatorMock{},
			},
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := connector{
				clientConnector: tc.fields.creator,
			}

			got, err := c.Connect(context.Background(), tc.args.mg)
			if tc.want.externalExists && got == nil {
				t.Errorf("expected external client, got nil")
			}
			expectedErrorBehaviour(t, tc.want.err, err)
		})
	}
}
func TestSaveCallback(t *testing.T) {
	type args struct {
		kube       client.Client
		name       string
		conditions []xpv1.Condition
	}

	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"GetError": {
			reason: "should return an error if the ServiceBinding cannot be retrieved",
			args: args{
				kube: &test.MockClient{MockGet: test.NewMockGetFn(errKube)},
				name: "test-instance",
			},
			want: want{
				err: errKube,
			},
		},
		"UpdateError": {
			reason: "should return an error if the ServiceBinding status cannot be updated",
			args: args{
				kube: &test.MockClient{
					MockGet:          test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(errKube),
				},
				name:       "test-instance",
				conditions: []xpv1.Condition{ujresource.AsyncOperationFinishedCondition()},
			},
			want: want{
				err: errKube,
			},
		},
		"Success": {
			reason: "should successfully save conditions to the ServiceBinding",
			args: args{
				kube: &test.MockClient{
					MockGet:          test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
				name:       "test-instance",
				conditions: []xpv1.Condition{ujresource.AsyncOperationFinishedCondition()},
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := saveCallback(context.Background(), tc.args.kube, tc.args.name, tc.args.conditions...)
			expectedErrorBehaviour(t, tc.want.err, err)
		})
	}
}

func TestDelete(t *testing.T) {
	type fields struct {
		client *TfProxyMock
	}
	type args struct {
		mg resource.Managed
	}
	type want struct {
		err error
		cr  *v1alpha1.ServiceBinding
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"ApiError": {
			reason: "should return an error when the API call fails",
			fields: fields{
				client: &TfProxyMock{err: errClient},
			},
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err: errClient,
				cr: buildExpectedServiceBinding(
					withConditions(xpv1.Deleting()),
				),
			},
		},
		"HappyPath": {
			reason: "should delete the resource successfully and set Deleting condition",
			fields: fields{
				client: &TfProxyMock{},
			},
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err: nil,
				cr: buildExpectedServiceBinding(
					withConditions(xpv1.Deleting()),
				),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				tfClient: tc.fields.client,
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
			}

			err := e.Delete(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)

			// Verify the entire CR
			cr, ok := tc.args.mg.(*v1alpha1.ServiceBinding)
			if !ok {
				t.Fatalf("expected *v1alpha1.ServiceBinding, got %T", tc.args.mg)
			}
			if diff := cmp.Diff(tc.want.cr, cr); diff != "" {
				t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type fields struct {
		client *TfProxyMock
	}
	type args struct {
		mg resource.Managed
	}
	type want struct {
		err error
		cr  *v1alpha1.ServiceBinding
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"ApiError": {
			reason: "should return an error when the API call fails",
			fields: fields{
				client: &TfProxyMock{err: errClient},
			},
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err: errClient,
				cr:  buildExpectedServiceBinding(),
			},
		},
		"HappyPath": {
			reason: "should update the resource successfully",
			fields: fields{
				client: &TfProxyMock{},
			},
			args: args{
				mg: &v1alpha1.ServiceBinding{},
			},
			want: want{
				err: nil,
				cr:  buildExpectedServiceBinding(),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				tfClient: tc.fields.client,
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
			}

			_, err := e.Update(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)

			// Verify the entire CR
			cr, ok := tc.args.mg.(*v1alpha1.ServiceBinding)
			if !ok {
				t.Fatalf("expected *v1alpha1.ServiceBinding, got %T", tc.args.mg)
			}
			if diff := cmp.Diff(tc.want.cr, cr); diff != "" {
				t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
			}
		})
	}
}

var _ tfClient.TfProxyConnectorI[*v1alpha1.ServiceBinding] = &TfProxyClientCreatorMock{}

type TfProxyClientCreatorMock struct {
	err error
}

func (t *TfProxyClientCreatorMock) Connect(ctx context.Context, cr *v1alpha1.ServiceBinding) (tfClient.TfProxyControllerI, error) {
	if t.err != nil {
		return nil, t.err
	}
	return &TfProxyMock{}, nil
}

var _ tfClient.TfProxyControllerI = &TfProxyMock{}

type TfProxyMock struct {
	status  tfclient.Status
	data    *tfClient.ObservationData
	err     error
	details map[string][]byte
}

func (t *TfProxyMock) QueryAsyncData(ctx context.Context) *tfClient.ObservationData {
	return t.data
}

func (t *TfProxyMock) Create(ctx context.Context) error {
	return t.err
}

func (t *TfProxyMock) Observe(context context.Context) (tfclient.Status, map[string][]byte, error) {
	return t.status, t.details, t.err
}

func (t *TfProxyMock) Delete(ctx context.Context) error {
	return t.err
}

func (t *TfProxyMock) Update(ctx context.Context) error {
	return t.err
}

func expectedErrorBehaviour(t *testing.T, expectedErr error, gotErr error) {
	if gotErr != nil {
		assert.Truef(t, errors.Is(gotErr, expectedErr), "expected error %v, got %v", expectedErr, gotErr)
		return
	}
	if expectedErr != nil {
		t.Errorf("expected error %v, got nil", expectedErr.Error())
	}
}

// Helper function to build a complete ServiceBinding CR dynamically
func buildExpectedServiceBinding(opts ...func(*v1alpha1.ServiceBinding)) *v1alpha1.ServiceBinding {
	cr := &v1alpha1.ServiceBinding{}

	// Apply each option to modify the CR
	for _, opt := range opts {
		opt(cr)
	}

	return cr
}

// Option to set the external name annotation
func withExternalName(externalName string) func(*v1alpha1.ServiceBinding) {
	return func(cr *v1alpha1.ServiceBinding) {
		if cr.GetAnnotations() == nil {
			cr.SetAnnotations(map[string]string{})
		}
		cr.GetAnnotations()["crossplane.io/external-name"] = externalName
	}
}

// Option to set observation data (e.g., ID)
func withObservationData(id string) func(*v1alpha1.ServiceBinding) {
	return func(cr *v1alpha1.ServiceBinding) {
		cr.Status.AtProvider = v1alpha1.ServiceBindingObservation{
			ID: id,
		}
	}
}

// Option to set conditions
func withConditions(conditions ...xpv1.Condition) func(*v1alpha1.ServiceBinding) {
	return func(cr *v1alpha1.ServiceBinding) {
		cr.Status.Conditions = conditions
	}
}
