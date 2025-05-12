package serviceinstance

import (
	"context"
	"errors"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	siClient "github.com/sap/crossplane-provider-btp/internal/clients/account/serviceinstance"
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
		cr  *v1alpha1.ServiceInstance // Expected complete CR
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
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: errClient,
				cr:  buildExpectedServiceInstance(), // No annotations, observation data, or conditions
			},
		},
		"NotFound": {
			reason: "should return not existing",
			fields: fields{
				client: &TfProxyMock{found: false},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				cr: buildExpectedServiceInstance(), // No annotations, observation data, or conditions
			},
		},
		"Happy, while async in process": {
			reason: "should return existing, but no data",
			fields: fields{
				client: &TfProxyMock{found: true},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: buildExpectedServiceInstance(), // No annotations, observation data, or conditions
			},
		},
		"Happy, no drift": {
			reason: "should return existing and pull data from embedded tf resource",
			fields: fields{
				client: &TfProxyMock{
					found: true,
					data: &siClient.ServiceInstanceData{
						ExternalName: "some-ext-name",
						ID:           "some-id",
					},
				},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: buildExpectedServiceInstance(
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
			cr, ok := tc.args.mg.(*v1alpha1.ServiceInstance)
			if !ok {
				t.Fatalf("expected *v1alpha1.ServiceInstance, got %T", tc.args.mg)
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
		cr  *v1alpha1.ServiceInstance // Expected complete CR after creation
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
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: errClient,
				cr: buildExpectedServiceInstance(
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
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
				cr: buildExpectedServiceInstance(
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
			cr, ok := tc.args.mg.(*v1alpha1.ServiceInstance)
			if !ok {
				t.Fatalf("expected *v1alpha1.ServiceInstance, got %T", tc.args.mg)
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
				mg: &v1alpha1.ServiceInstance{},
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
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := connector{
				newClientCreatorFn: func(_ client.Client) siClient.TfProxyClientCreator { return tc.fields.creator },
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
			reason: "should return an error if the ServiceInstance cannot be retrieved",
			args: args{
				kube: &test.MockClient{MockGet: test.NewMockGetFn(errKube)},
				name: "test-instance",
			},
			want: want{
				err: errKube,
			},
		},
		"UpdateError": {
			reason: "should return an error if the ServiceInstance status cannot be updated",
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
			reason: "should successfully save conditions to the ServiceInstance",
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
		cr  *v1alpha1.ServiceInstance
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
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: errClient,
				cr: buildExpectedServiceInstance(
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
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: nil,
				cr: buildExpectedServiceInstance(
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
			cr, ok := tc.args.mg.(*v1alpha1.ServiceInstance)
			if !ok {
				t.Fatalf("expected *v1alpha1.ServiceInstance, got %T", tc.args.mg)
			}
			if diff := cmp.Diff(tc.want.cr, cr); diff != "" {
				t.Errorf("\n%s\nCR mismatch (-want, +got):\n%s\n", tc.reason, diff)
			}
		})
	}
}

var _ siClient.TfProxyClientCreator = &TfProxyClientCreatorMock{}

type TfProxyClientCreatorMock struct {
	err error
}

func (t *TfProxyClientCreatorMock) Connect(ctx context.Context, cr *v1alpha1.ServiceInstance) (siClient.TfProxyClient, error) {
	if t.err != nil {
		return nil, t.err
	}
	return &TfProxyMock{}, nil
}

var _ siClient.TfProxyClient = &TfProxyMock{}

type TfProxyMock struct {
	found bool
	data  *siClient.ServiceInstanceData
	err   error
}

func (t *TfProxyMock) QueryAsyncData(ctx context.Context) *siClient.ServiceInstanceData {
	return t.data
}

func (t *TfProxyMock) Create(ctx context.Context) error {
	return t.err
}

func (t *TfProxyMock) Observe(context context.Context) (bool, error) {
	return t.found, t.err
}

func (t *TfProxyMock) Delete(ctx context.Context) error {
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

// Helper function to build a complete ServiceInstance CR dynamically
func buildExpectedServiceInstance(opts ...func(*v1alpha1.ServiceInstance)) *v1alpha1.ServiceInstance {
	cr := &v1alpha1.ServiceInstance{}

	// Apply each option to modify the CR
	for _, opt := range opts {
		opt(cr)
	}

	return cr
}

// Option to set the external name annotation
func withExternalName(externalName string) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		if cr.GetAnnotations() == nil {
			cr.SetAnnotations(map[string]string{})
		}
		cr.GetAnnotations()["crossplane.io/external-name"] = externalName
	}
}

// Option to set observation data (e.g., ID)
func withObservationData(id string) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Status.AtProvider = v1alpha1.ServiceInstanceObservation{
			ID: id,
		}
	}
}

// Option to set conditions
func withConditions(conditions ...xpv1.Condition) func(*v1alpha1.ServiceInstance) {
	return func(cr *v1alpha1.ServiceInstance) {
		cr.Status.Conditions = conditions
	}
}
