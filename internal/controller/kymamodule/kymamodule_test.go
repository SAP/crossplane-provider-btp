package kymamodule

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymamodule"
	"github.com/sap/crossplane-provider-btp/internal/controller/kymamodule/fake"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestObserve(t *testing.T) {
	type args struct {
		cr            resource.Managed
		client        kymamodule.Client
		kube          client.Client
		tracker       tracking.ReferenceResolverTracker
		secretfetcher SecretFetcherInterface
	}

	type want struct {
		obs managed.ExternalObservation
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"HappyPath": {
			args: args{
				cr: module(withBindingRef("test-binding")),
				client: &fake.MockKymaModuleClient{
					MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
						return &v1alpha1.ModuleStatus{}, nil
					},
				},
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
						if binding, ok := obj.(*v1alpha1.KymaEnvironmentBinding); ok {
							binding.SetName("test-binding")
							binding.SetNamespace("default")
						}
						return nil
					}),
				},
				tracker: &fake.MockTracker{},
				secretfetcher: &fake.MockSecretFetcher{
					MockFetch: func(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error) {
						return []byte("VALID KUBECONFIG"), nil
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				err: nil,
			},
		},
		"NeedsCreation": {
			args: args{
				cr: module(withBindingRef("test-binding")),
				client: &fake.MockKymaModuleClient{
					MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
						return nil, nil
					},
				},
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
						if binding, ok := obj.(*v1alpha1.KymaEnvironmentBinding); ok {
							binding.SetName("test-binding")
							binding.SetNamespace("default")
						}
						return nil
					}),
				},
				tracker: &fake.MockTracker{},
				secretfetcher: &fake.MockSecretFetcher{
					MockFetch: func(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error) {
						return []byte("VALID KUBECONFIG"), nil
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{ResourceExists: false},
				err: nil,
			},
		},
		"ApiNotAvailable": {
			args: args{
				cr: module(withBindingRef("test-binding")),
				client: &fake.MockKymaModuleClient{
					MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
						return nil, errors.New("CRASH")
					},
				},
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
						if binding, ok := obj.(*v1alpha1.KymaEnvironmentBinding); ok {
							binding.SetName("test-binding")
							binding.SetNamespace("default")
						}
						return nil
					}),
				},
				tracker: &fake.MockTracker{},
				secretfetcher: &fake.MockSecretFetcher{
					MockFetch: func(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error) {
						return []byte("VALID KUBECONFIG"), nil
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{},
				err: errors.Wrap(errors.New("CRASH"), errObserveResource),
			},
		},
		"BindingNotFound": {
			args: args{
				cr: module(withBindingRef("missing-binding")),
				client: &fake.MockKymaModuleClient{
					MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
						return &v1alpha1.ModuleStatus{}, nil
					},
				},
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(kerrors.NewNotFound(
						schema.GroupResource{Group: "environment.btp.sap.crossplane.io", Resource: "kymaenvironmentbindings"},
						"missing-binding",
					)),
				},
				tracker: &fake.MockTracker{},
				secretfetcher: &fake.MockSecretFetcher{
					MockFetch: func(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error) {
						return []byte("VALID KUBECONFIG"), nil
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{ResourceExists: false},
				err: nil,
			},
		},
		"BindingBeingDeleted": {
			args: args{
				cr: module(withBindingRef("deleting-binding")),
				client: &fake.MockKymaModuleClient{
					MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
						return &v1alpha1.ModuleStatus{}, nil
					},
				},
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
						if binding, ok := obj.(*v1alpha1.KymaEnvironmentBinding); ok {
							binding.SetName("deleting-binding")
							binding.SetNamespace("default")
							now := metav1.Now()
							binding.SetDeletionTimestamp(&now)
						}
						return nil
					}),
				},
				tracker: &fake.MockTracker{},
				secretfetcher: &fake.MockSecretFetcher{
					MockFetch: func(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error) {
						return []byte("VALID KUBECONFIG"), nil
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				err: nil,
			},
		},
		"TrackerError": {
			args: args{
				cr: module(withBindingRef("test-binding")),
				client: &fake.MockKymaModuleClient{
					MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
						return &v1alpha1.ModuleStatus{}, nil
					},
				},
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
						if binding, ok := obj.(*v1alpha1.KymaEnvironmentBinding); ok {
							binding.SetName("test-binding")
							binding.SetNamespace("default")
						}
						return nil
					}),
				},
				tracker: &fake.MockTracker{
					MockTrack: func(ctx context.Context, mg resource.Managed) error {
						return errors.New("tracker failed")
					},
				},
				secretfetcher: &fake.MockSecretFetcher{
					MockFetch: func(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error) {
						return []byte("VALID KUBECONFIG"), nil
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{},
				err: errors.Wrap(errors.New("tracker failed"), errTrackRUsage),
			},
		},
		"TrackerCalledWithCorrectResource": {
			args: args{
				cr: module(withBindingRef("test-binding")),
				client: &fake.MockKymaModuleClient{
					MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
						return &v1alpha1.ModuleStatus{}, nil
					},
				},
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
						if binding, ok := obj.(*v1alpha1.KymaEnvironmentBinding); ok {
							binding.SetName("test-binding")
							binding.SetNamespace("default")
						}
						return nil
					}),
				},
				tracker: &fake.MockTracker{
					MockTrack: func(ctx context.Context, mg resource.Managed) error {
						// Verify Track is called with the correct resource
						km, ok := mg.(*v1alpha1.KymaModule)
						if !ok {
							return errors.New("expected KymaModule, got different type")
						}
						if km.GetName() != "kymaModule" {
							return errors.New("unexpected KymaModule name")
						}
						return nil
					},
				},
				secretfetcher: &fake.MockSecretFetcher{
					MockFetch: func(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error) {
						return []byte("VALID KUBECONFIG"), nil
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				client:        tc.args.client,
				kube:          tc.args.kube,
				tracker:       tc.args.tracker,
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
		obs managed.ExternalCreation
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"HappyPath": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{
					MockCreate: func(moduleName string, moduleChannel string, customResourcePolicy string) error {
						return nil
					},
				},
			},
			want: want{
				obs: managed.ExternalCreation{},
				err: nil,
			},
		},
		"ApiNotAvailable": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{
					MockCreate: func(moduleName string, moduleChannel string, customResourcePolicy string) error {
						return errors.New("CRASH")
					},
				},
			},
			want: want{
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
				t.Errorf("\ne.Create(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.obs, got); diff != "" {
				t.Errorf("\ne.Create(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		cr     resource.Managed
		client kymamodule.Client
		kube   client.Client
	}

	type want struct {
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"HappyPath": {
			args: args{
				cr: module(withBindingRef("test-binding")),
				client: &fake.MockKymaModuleClient{
					MockDelete: func(moduleName string) error {
						return nil
					},
				},
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil),
				},
			},
			want: want{
				err: nil,
			},
		},
		"ApiNotAvailable": {
			args: args{
				cr: module(withBindingRef("test-binding")),
				client: &fake.MockKymaModuleClient{
					MockDelete: func(moduleName string) error {
						return errors.New("CRASH")
					},
				},
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil),
				},
			},
			want: want{
				err: errors.New("CRASH"),
			},
		},
		"BindingAlreadyDeleted": {
			args: args{
				cr: module(withBindingRef("missing-binding")),
				client: &fake.MockKymaModuleClient{
					MockDelete: func(moduleName string) error {
						return nil
					},
				},
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(kerrors.NewNotFound(
						schema.GroupResource{Group: "environment.btp.sap.crossplane.io", Resource: "kymaenvironmentbindings"},
						"missing-binding",
					)),
				},
			},
			want: want{
				err: nil,
			},
		},
		"NilClient": {
			args: args{
				cr:     module(withBindingRef("test-binding")),
				client: nil,
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil),
				},
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{
				client: tc.args.client,
				kube:   tc.args.kube,
			}
			_, err := e.Delete(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Delete(...): -want error, +got error:\n%s\n", diff)
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
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "HappyPath",
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
			name: "ExpiredSecret",
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
			name: "InvalidKubeconfig",
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
			name: "InvalidExpirationFormat",
			args: args{
				secret: map[string][]byte{
					v1alpha1.KymaEnvironmentBindingKey:           []byte("VALID KUBECONFIG DATA"),
					v1alpha1.KymaEnvironmentBindingExpirationKey: []byte("INVALID DATE FORMAT"),
				},
			},
			want: want{
				wantErr:    errors.New(errTimeParser),
				wantConfig: nil,
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
			Name:      "kymaModule",
			Namespace: "default",
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

func withBindingRef(name string) moduleModifier {
	return func(km *v1alpha1.KymaModule) {
		km.Spec.KymaEnvironmentBindingRef = &xpv1.Reference{
			Name: name,
		}
	}
}

func ptrString(s string) *string {
	return &s
}
