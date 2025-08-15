package kymamodule

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymamodule"
	"github.com/sap/crossplane-provider-btp/internal/controller/kymamodule/fake"
)

func TestObserve(t *testing.T) {

	type args struct {
		cr            resource.Managed
		client        kymamodule.Client
		secretfetcher SecretFetcherInterface
		newService    func(kymaEnvironmentKubeconfig []byte) (kymamodule.Client, error)
	}

	type want struct {
		cr  resource.Managed
		obs managed.ExternalObservation
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"Happy Path": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
					return &v1alpha1.ModuleStatus{}, nil
				}},
				secretfetcher: &fake.MockSecretFetcher{MockFetch: func(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error) {
					return []byte("VALID KUBECONFIG"), nil
				}},
			},
			want: want{
				cr:  module(),
				obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				err: nil,
			},
		},
		"Needs Creation": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
					return nil, nil
				}},
				secretfetcher: &fake.MockSecretFetcher{MockFetch: func(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error) {
					return []byte("VALID KUBECONFIG"), nil
				}},
			},
			want: want{
				cr:  module(),
				obs: managed.ExternalObservation{ResourceExists: false},
				err: nil,
			},
		},
		"Api Not Available": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
					return nil, errors.New("CRASH")
				}},
				newService: func(kymaEnvironmentKubeconfig []byte) (kymamodule.Client, error) {
					return nil, nil
				},
				secretfetcher: &fake.MockSecretFetcher{MockFetch: func(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error) {
					return []byte("VALID KUBECONFIG"), nil
				}},
			},
			want: want{
				cr:  module(),
				obs: managed.ExternalObservation{},
				err: errors.Wrap(errors.New("CRASH"), errObserveResource),
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				client:        tc.args.client,
				secretfetcher: tc.args.secretfetcher,
			}
			got, err := e.Observe(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.obs, got); diff != "" {
				t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type args struct {
		cr     resource.Managed
		client kymamodule.Client
	}

	type want struct {
		cr  resource.Managed
		obs managed.ExternalCreation
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"Happy Path": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{MockCreate: func(moduleName string, moduleChannel string, customResourcePolicy string) error {
					return nil
				}},
			},
			want: want{
				cr:  module(),
				obs: managed.ExternalCreation{},
				err: nil,
			},
		},
		"Api Not Available": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{MockCreate: func(moduleName string, moduleChannel string, customResourcePolicy string) error {
					return errors.New("CRASH")
				}},
			},
			want: want{
				cr:  module(),
				obs: managed.ExternalCreation{},
				err: errors.New("CRASH"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			got, err := e.Create(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.obs, got); diff != "" {
				t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		cr     resource.Managed
		client kymamodule.Client
	}

	type want struct {
		cr  resource.Managed
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"Happy Path": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{MockDelete: func(moduleName string) error {
					return nil
				}},
			},
			want: want{
				cr:  module(),
				err: nil,
			},
		},
		"Api Not Available": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{MockDelete: func(moduleName string) error {
					return errors.New("CRASH")
				}},
			},
			want: want{
				cr:  module(),
				err: errors.New("CRASH"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			_, err := e.Delete(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}

func TestGetValidKubeconfig(t *testing.T) {
	type args struct {
		secret map[string][]byte
	}

	type want struct {
		wantErr    error
		wantConfig []byte
		wantValid  bool
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "Happy Path",
			args: args{
				secret: map[string][]byte{
					v1alpha1.KymaEnvironmentBindingKey:           []byte("VALID KUBECONFIG DATA"),
					v1alpha1.KymaEnvironmentBindingExpirationKey: []byte("9999-09-09 00:00:00 +0000 UTC"),
				},
			},
			want: want{
				wantErr:    nil,
				wantConfig: []byte("VALID KUBECONFIG DATA"),
			},
		},
		{
			name: "Expired Secret",
			args: args{
				secret: map[string][]byte{
					v1alpha1.KymaEnvironmentBindingKey:           []byte("VALID KUBECONFIG DATA"),
					v1alpha1.KymaEnvironmentBindingExpirationKey: []byte("2020-01-01 00:00:00 +0000 UTC"),
				},
			},
			want: want{
				wantErr:    nil,
				wantConfig: nil,
			},
		},
		{
			name: "Invalid Kubeconfig",
			args: args{
				secret: map[string][]byte{
					v1alpha1.KymaEnvironmentBindingKey:           []byte(""),
					v1alpha1.KymaEnvironmentBindingExpirationKey: []byte("9999-09-09 00:00:00 +0000 UTC"),
				},
			},
			want: want{
				wantErr:    errors.New(errCredentialsCorrupted),
				wantConfig: nil,
			},
		},
		{
			name: "Invalid Expiration Format",
			args: args{
				secret: map[string][]byte{
					v1alpha1.KymaEnvironmentBindingKey:           []byte("VALID KUBECONFIG DATA"),
					v1alpha1.KymaEnvironmentBindingExpirationKey: []byte("INVALID DATE FORMAT"),
				},
			},
			want: want{
				wantErr:    errors.New(errTimeParser),
				wantConfig: nil,
				wantValid:  false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeconfig, err := getValidKubeconfig(tt.args.secret)

			if diff := cmp.Diff(tt.want.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ngetValidKubeconfig(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tt.want.wantConfig, kubeconfig); diff != "" {
				t.Errorf("\ngetValidKubeconfig(...): -want kubeconfig, +got kubeconfig:\n%s\n", diff)
			}
		})
	}

}

type moduleModifier func(kymaModule *v1alpha1.KymaModule)

func module(m ...moduleModifier) *v1alpha1.KymaModule {
	cr := &v1alpha1.KymaModule{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kymaModule",
		},
		Spec: v1alpha1.KymaModuleSpec{
			ForProvider: v1alpha1.KymaModuleParameters{
				Name:                 "testModule",
				Channel:              ptrString("regular"),
				CustomResourcePolicy: ptrString("createdelete"),
			},
		},
	}

	for _, f := range m {
		f(cr)
	}
	return cr
}

func ptrString(s string) *string {
	return &s
}
