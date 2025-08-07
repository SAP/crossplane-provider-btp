package servicemanager

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	"github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	test2 "github.com/sap/crossplane-provider-btp/internal/tracking/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestObserve(t *testing.T) {
	type want struct {
		err error
		obs managed.ExternalObservation
		cr  *v1beta1.ServiceManager
	}
	type args struct {
		cr       *v1beta1.ServiceManager
		tfClient *TfClientFake
	}
	tests := []struct {
		name string
		args args

		want want
	}{
		{
			name: "InstanceObserveError",
			args: args{
				cr: NewServiceManager("test"),
				tfClient: &TfClientFake{
					observeFn: func() (servicemanager.ResourcesStatus, error) {
						return servicemanager.ResourcesStatus{}, errors.New("observeError")
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{},
				err: errors.New("observeError"),
				cr: NewServiceManager("test",
					WithStatus(v1beta1.ServiceManagerObservation{
						Status: v1beta1.ServiceManagerUnbound,
					}),
					WithConditions(xpv1.Unavailable())),
			},
		},
		{
			name: "NotAvailable",
			args: args{
				cr: NewServiceManager("test"),
				tfClient: &TfClientFake{
					observeFn: func() (servicemanager.ResourcesStatus, error) {
						// Doesn't matter what observe is returned exactly, as long as its passed through and IDs are persisted
						return servicemanager.ResourcesStatus{
							ExternalObservation: managed.ExternalObservation{ResourceExists: false},
							InstanceID:          "someID",
						}, nil
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{ResourceExists: false},
				err: nil,
				cr: NewServiceManager("test",
					WithStatus(v1beta1.ServiceManagerObservation{
						Status:            v1beta1.ServiceManagerUnbound,
						ServiceInstanceID: "someID",
					}),
					WithConditions(xpv1.Unavailable()),
				),
			},
		},
		{
			name: "IsAvailable",
			args: args{
				cr: NewServiceManager("test"),
				tfClient: &TfClientFake{
					observeFn: func() (servicemanager.ResourcesStatus, error) {
						// Doesn't matter if updated or not
						return servicemanager.ResourcesStatus{
							ExternalObservation: managed.ExternalObservation{
								ResourceExists:    true,
								ResourceUpToDate:  true,
								ConnectionDetails: map[string][]byte{"key": []byte("value")},
							},
							InstanceID: "someID",
							BindingID:  "anotherID",
						}, nil

					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: map[string][]byte{"key": []byte("value")}},
				err: nil,
				cr: NewServiceManager("test",
					WithStatus(v1beta1.ServiceManagerObservation{
						Status:            v1beta1.ServiceManagerBound,
						ServiceInstanceID: "someID",
						ServiceBindingID:  "anotherID",
					}),
					WithConditions(xpv1.Available())),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &external{
				tfClient: tc.args.tfClient,
				kube: &test.MockClient{
					MockStatusUpdate: func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
						return nil
					},
				},
			}
			obs, err := uua.Observe(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(obs, tc.want.obs); diff != "" {
				t.Errorf("\ne.Observe(): -want, +got:\n%s\n", diff)
			}
			if diff := cmp.Diff(err, tc.want.err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.args.cr, tc.want.cr); diff != "" {
				t.Errorf("\ne.Observe(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type want struct {
		err error
		cr  *v1beta1.ServiceManager
	}
	type args struct {
		cr       *v1beta1.ServiceManager
		tfClient *TfClientFake
	}
	tests := []struct {
		name string
		args args

		want want
	}{
		{
			name: "CreateError",
			args: args{
				cr: NewServiceManager("test"),
				tfClient: &TfClientFake{
					createFn: func() (string, string, error) {
						return "", "", errors.New("createError")
					},
				},
			},
			want: want{
				err: errors.New("createError"),
				cr:  NewServiceManager("test", WithConditions(xpv1.Creating())),
			},
		},
		{
			name: "Success",
			args: args{
				cr: NewServiceManager("test"),
				tfClient: &TfClientFake{
					createFn: func() (string, string, error) {
						return "someID", "anotherID", nil
					},
				},
			},
			want: want{
				err: nil,
				cr: NewServiceManager("test",
					WithExternalName("someID/anotherID"),
					WithConditions(xpv1.Creating()),
				),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &external{
				tfClient: tc.args.tfClient,
			}
			_, err := uua.Create(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Create(): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\ne.Create(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type want struct {
		err error
	}
	type args struct {
		cr       *v1beta1.ServiceManager
		tfClient *TfClientFake
	}
	tests := []struct {
		name string
		args args

		want want
	}{
		{
			name: "UpdateError",
			args: args{
				cr: NewServiceManager("test", WithExternalName("someID")),
				tfClient: &TfClientFake{
					updateFn: func() error {
						return errors.New("updateError")
					},
				},
			},
			want: want{
				err: errors.New("updateError"),
			},
		},
		{
			name: "Success",
			args: args{
				cr: NewServiceManager("test", WithExternalName("someID")),
				tfClient: &TfClientFake{
					updateFn: func() error {
						return nil
					},
				},
			},
			want: want{
				err: nil,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &external{
				tfClient: tc.args.tfClient,
			}
			_, err := uua.Update(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(err, tc.want.err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Update(): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type want struct {
		err error
		cr  *v1beta1.ServiceManager
	}
	type args struct {
		cr       *v1beta1.ServiceManager
		tfClient *TfClientFake
	}
	tests := []struct {
		name string
		args args

		want want
	}{
		{
			name: "DeleteError",
			args: args{
				cr: NewServiceManager("test", WithExternalName("someID/anotherID")),
				tfClient: &TfClientFake{
					deleteFn: func() error {
						return errors.New("deleteError")
					},
				},
			},
			want: want{
				err: errors.New("deleteError"),
				cr:  NewServiceManager("test", WithExternalName("someID/anotherID"), WithConditions(xpv1.Deleting())),
			},
		},
		{
			name: "Success",
			args: args{
				cr: NewServiceManager("test", WithExternalName("someID/anotherID")),
				tfClient: &TfClientFake{
					deleteFn: func() error {
						return nil
					},
				},
			},
			want: want{
				err: nil,
				cr:  NewServiceManager("test", WithExternalName("someID/anotherID"), WithConditions(xpv1.Deleting())),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &external{
				tracker:  test2.NoOpReferenceResolverTracker{},
				tfClient: tc.args.tfClient,
			}
			_, err := uua.Delete(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Delete(): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\ne.Delete(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

// Utils
func NewServiceManager(name string, m ...ServiceManagerModifier) *v1beta1.ServiceManager {
	cr := &v1beta1.ServiceManager{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	meta.SetExternalName(cr, name)
	for _, f := range m {
		f(cr)
	}
	return cr
}

// this pattern can be potentially auto generated, its quite useful to write expressive unittests
type ServiceManagerModifier func(dirEnvironment *v1beta1.ServiceManager)

func WithStatus(status v1beta1.ServiceManagerObservation) ServiceManagerModifier {
	return func(r *v1beta1.ServiceManager) {
		r.Status.AtProvider = status
	}
}

func WithData(data v1beta1.ServiceManagerParameters) ServiceManagerModifier {
	return func(r *v1beta1.ServiceManager) {
		r.Spec.ForProvider = data
	}
}

func WithConditions(c ...xpv1.Condition) ServiceManagerModifier {
	return func(r *v1beta1.ServiceManager) { r.Status.ConditionedStatus.Conditions = c }
}

func WithExternalName(externalName string) ServiceManagerModifier {
	return func(r *v1beta1.ServiceManager) {
		meta.SetExternalName(r, externalName)
	}
}

// Fakes
var _ servicemanager.ITfClient = &TfClientFake{}

type TfClientFake struct {
	observeFn func() (servicemanager.ResourcesStatus, error)
	createFn  func() (string, string, error)
	updateFn  func() error
	deleteFn  func() error
}

func (t TfClientFake) ObserveResources(ctx context.Context, cr *v1beta1.ServiceManager) (servicemanager.ResourcesStatus, error) {
	return t.observeFn()
}

func (t TfClientFake) CreateResources(ctx context.Context, cr *v1beta1.ServiceManager) (string, string, error) {
	return t.createFn()
}

func (t TfClientFake) UpdateResources(ctx context.Context, cr *v1beta1.ServiceManager) error {
	return t.updateFn()
}

func (t TfClientFake) DeleteResources(ctx context.Context, cr *v1beta1.ServiceManager) error {
	return t.deleteFn()
}
