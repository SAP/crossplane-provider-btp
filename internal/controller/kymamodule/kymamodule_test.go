package kymamodule

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
		cr     resource.Managed
		client kymamodule.Client
	}

	type want struct {
		cr  resource.Managed
		obs managed.ExternalObservation
		err error
	}

	cases := map[string]struct {
		args       args
		want       want
		secretData map[string][]byte
	}{
		"Happy Path": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
					return &v1alpha1.ModuleStatus{}, nil
				}},
			},
			want: want{
				cr:  module(),
				obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				err: nil,
			},
			secretData: expiredSecretData(),
		},
		"Needs Creation": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
					return nil, nil
				}},
			},
			want: want{
				cr:  module(),
				obs: managed.ExternalObservation{ResourceExists: false},
				err: nil,
			},
			secretData: expiredSecretData(),
		},
		"Boom!": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
					return nil, errors.New("BOOM")
				}},
			},
			want: want{
				cr:  module(),
				obs: managed.ExternalObservation{},
				err: errors.Wrap(errors.New("BOOM"), errObserveResource),
			},
			secretData: expiredSecretData(),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mockGet := func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				secret, _ := obj.(*corev1.Secret)
				secret.Data = tc.secretData
				return nil
			}
			e := external{
				client: tc.args.client,
				kube:   &test.MockClient{MockGet: mockGet},
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
		"Boom!": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{MockCreate: func(moduleName string, moduleChannel string, customResourcePolicy string) error {
					return errors.New("BOOM")
				}},
			},
			want: want{
				cr:  module(),
				obs: managed.ExternalCreation{},
				err: errors.New("BOOM"),
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
		"Boom!": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{MockDelete: func(moduleName string) error {
					return errors.New("BOOM")
				}},
			},
			want: want{
				cr:  module(),
				err: errors.New("BOOM"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			err := e.Delete(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}

func TestGetKubeconfig(t *testing.T) {
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
				secret: validSecretData(),
			},
			want: want{
				wantErr:    nil,
				wantConfig: []byte("VALID KUBECONFIG DATA"),
			},
		},
		{
			name: "Expired Secret",
			args: args{
				secret: expiredSecretData(),
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
			kubeconfig, err := getKubeconfig(tt.args.secret)

			if diff := cmp.Diff(tt.want.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ngetKubeconfig(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tt.want.wantConfig, kubeconfig); diff != "" {
				t.Errorf("\ngetKubeconfig(...): -want kubeconfig, +got kubeconfig:\n%s\n", diff)
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

			KymaEnvironmentBindingSecret:          "test-binding-secret",
			KymaEnvironmentBindingSecretNamespace: "test-namespace",
		},
	}

	for _, f := range m {
		f(cr)
	}
	return cr
}

func expiredSecretData() map[string][]byte {
	return map[string][]byte{
		v1alpha1.KymaEnvironmentBindingKey:           []byte("VALID KUBECONFIG DATA"),
		v1alpha1.KymaEnvironmentBindingExpirationKey: []byte("2020-01-01 00:00:00 +0000 UTC"),
	}
}

func validSecretData() map[string][]byte {
	return map[string][]byte{
		v1alpha1.KymaEnvironmentBindingKey:           []byte("VALID KUBECONFIG DATA"),
		v1alpha1.KymaEnvironmentBindingExpirationKey: []byte("9999-09-09 00:00:00 +0000 UTC"),
	}
}

func ptrString(s string) *string {
	return &s
}
