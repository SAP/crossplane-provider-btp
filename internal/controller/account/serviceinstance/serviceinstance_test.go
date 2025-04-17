package serviceinstance

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/stretchr/testify/assert"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
)

var errApi = errors.New("apiError")

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
				client: &TfProxyMock{err: errApi},
			},
			args: args{
				mg: &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: errApi,
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
			},
		},
		"Happy, no drift": {
			reason: "should return existing and pull data from embedded tf resource",
			fields: fields{
				client: &TfProxyMock{
					found: true,
					data: &ServiceInstanceData{
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
		})
	}
}

func TestCreate(t *testing.T) {
	type want struct {
		err error
	}
	type args struct {
		client *TfProxyMock
		mg     *v1alpha1.ServiceInstance
	}
	cases := map[string]struct {
		args args
		want want
	}{
		"ApiError": {
			args: args{
				client: &TfProxyMock{err: errApi},
				mg:     &v1alpha1.ServiceInstance{},
			},
			want: want{
				err: errApi,
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{tfClient: tc.args.client}
			_, err := e.Create(context.Background(), tc.args.mg)
			expectedErrorBehaviour(t, tc.want.err, err)
		})
	}
}

var _ TfProxyClient = &TfProxyMock{}

type TfProxyMock struct {
	found bool
	data  *ServiceInstanceData
	err   error
}

// QueryAsyncData implements TfProxyClient.
func (t *TfProxyMock) QueryAsyncData(ctx context.Context, cr *v1alpha1.ServiceInstance) *ServiceInstanceData {
	return t.data
}

// Create implements TfProxyClient.
func (t *TfProxyMock) Create(ctx context.Context, cr *v1alpha1.ServiceInstance) error {
	return t.err
}

// Observe implements TfProxyClient.
func (t *TfProxyMock) Observe(context context.Context, cr *v1alpha1.ServiceInstance) (bool, error) {
	return t.found, t.err
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
