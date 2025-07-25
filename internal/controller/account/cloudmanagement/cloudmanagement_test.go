package cloudmanagement

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	"github.com/sap/crossplane-provider-btp/internal"
	cmclient "github.com/sap/crossplane-provider-btp/internal/clients/cis"
	"github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	test2 "github.com/sap/crossplane-provider-btp/internal/tracking/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/test"
)

func TestConnect(t *testing.T) {
	type want struct {
		err error
		cr  *v1beta1.CloudManagement
	}
	type args struct {
		cr                  *v1beta1.CloudManagement
		kube                test.MockClient
		planIdResolverFn    func(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error)
		clientInitializerFn func() cmclient.ITfClientInitializer
	}
	tests := []struct {
		name string
		args args

		want want
	}{
		{
			name: "NoServiceManagerSecretSpec",
			args: args{
				cr: NewCloudManagement("test"),
			},
			want: want{
				err: errors.New(errExtractSecretKey),
				cr:  NewCloudManagement("test"),
			},
		},
		{
			name: "ServiceManagerSecretNotFound",
			args: args{
				kube: test.MockClient{
					MockGet: test.NewMockGetFn(errors.New("GetSecretError")),
				},
				cr: NewCloudManagement("test",
					WithData(v1beta1.CloudManagementParameters{
						ServiceManagerSecretNamespace: "someNamespace",
						ServiceManagerSecret:          "someSecret",
					}),
				),
			},
			want: want{
				err: errors.Wrap(errors.New("GetSecretError"), errGetCredentialsSecret),
				cr: NewCloudManagement("test",
					WithData(v1beta1.CloudManagementParameters{
						ServiceManagerSecretNamespace: "someNamespace",
						ServiceManagerSecret:          "someSecret",
					}),
				),
			},
		},
		{
			name: "PlanIdResolverInitError",
			args: args{
				kube: test.MockClient{
					MockGet:          test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
				cr: NewCloudManagement("test",
					WithData(v1beta1.CloudManagementParameters{
						ServiceManagerSecretNamespace: "someNamespace",
						ServiceManagerSecret:          "someSecret",
					}),
				),
				planIdResolverFn: func(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error) {
					return nil, errors.New("ResolverInitError")
				},
			},
			want: want{
				err: errors.New("ResolverInitError"),
				cr: NewCloudManagement("test",
					WithData(v1beta1.CloudManagementParameters{
						ServiceManagerSecretNamespace: "someNamespace",
						ServiceManagerSecret:          "someSecret",
					}),
				),
			},
		},
		{
			name: "IntializeEmptyResource",
			args: args{
				kube: test.MockClient{
					MockGet:          test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
				cr: NewCloudManagement("test",
					WithData(v1beta1.CloudManagementParameters{
						ServiceManagerSecretNamespace: "someNamespace",
						ServiceManagerSecret:          "someSecret",
					}),
				),
				planIdResolverFn: func(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error) {
					return PlanIDFake{
						func(ctx context.Context, offeringName string, servicePlanName string) (string, error) {
							return "planID", nil
						},
					}, nil
				},
				clientInitializerFn: func() cmclient.ITfClientInitializer {
					return &ClientInitializerFake{
						ConnectResourcesFn: func(ctx context.Context, cr *v1beta1.CloudManagement) (cmclient.ITfClient, error) {
							return &TfClientFake{}, nil
						},
					}
				},
			},
			want: want{
				err: nil,
				cr: NewCloudManagement("test",
					WithData(v1beta1.CloudManagementParameters{
						ServiceManagerSecretNamespace: "someNamespace",
						ServiceManagerSecret:          "someSecret",
					}),
					WithStatus(v1beta1.CloudManagementObservation{
						DataSourceLookup: &v1beta1.CloudManagementDataSourceLookup{
							CloudManagementPlanID: "planID",
						},
					}),
				),
			},
		},
		{
			name: "AlreadyInitialized",
			args: args{
				kube: test.MockClient{
					MockGet:          test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
				},
				cr: NewCloudManagement("test",
					WithData(v1beta1.CloudManagementParameters{
						ServiceManagerSecretNamespace: "someNamespace",
						ServiceManagerSecret:          "someSecret",
					}),
					WithStatus(v1beta1.CloudManagementObservation{
						DataSourceLookup: &v1beta1.CloudManagementDataSourceLookup{
							CloudManagementPlanID: "planID",
						},
					}),
				),
				planIdResolverFn: func(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error) {
					return PlanIDFake{
						func(ctx context.Context, offeringName string, servicePlanName string) (string, error) {
							return "planID", nil
						},
					}, nil
				},
				clientInitializerFn: func() cmclient.ITfClientInitializer {
					return &ClientInitializerFake{
						ConnectResourcesFn: func(ctx context.Context, cr *v1beta1.CloudManagement) (cmclient.ITfClient, error) {
							return &TfClientFake{}, nil
						},
					}
				},
			},
			want: want{
				err: nil,
				cr: NewCloudManagement("test",
					WithData(v1beta1.CloudManagementParameters{
						ServiceManagerSecretNamespace: "someNamespace",
						ServiceManagerSecret:          "someSecret",
					}),
					WithStatus(v1beta1.CloudManagementObservation{
						DataSourceLookup: &v1beta1.CloudManagementDataSourceLookup{
							CloudManagementPlanID: "planID",
						},
					}),
				),
			},
		},
		{
			// we changed the approach from using the API to using tf resources internally, we have to ensure some smooth migration
			name: "MigrateFromPreviousVersion",
			args: args{
				kube: test.MockClient{
					MockGet:          test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
					MockUpdate:       test.NewMockUpdateFn(nil),
				},
				cr: NewCloudManagement("test",
					WithExternalName("crName"),
					WithData(v1beta1.CloudManagementParameters{
						ServiceManagerSecretNamespace: "someNamespace",
						ServiceManagerSecret:          "someSecret",
					}),
					WithStatus(v1beta1.CloudManagementObservation{
						Instance: &v1beta1.Instance{
							Id: internal.Ptr("someID"),
						},
						Binding: &v1beta1.Binding{
							Id: internal.Ptr("anotherID"),
						},
					}),
				),
				planIdResolverFn: func(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error) {
					return PlanIDFake{
						func(ctx context.Context, offeringName string, servicePlanName string) (string, error) {
							return "planID", nil
						},
					}, nil
				},
				clientInitializerFn: func() cmclient.ITfClientInitializer {
					return &ClientInitializerFake{
						ConnectResourcesFn: func(ctx context.Context, cr *v1beta1.CloudManagement) (cmclient.ITfClient, error) {
							return &TfClientFake{}, nil
						},
					}
				},
			},
			want: want{
				err: nil,
				cr: NewCloudManagement("test",
					WithExternalName("someID/anotherID"),
					WithData(v1beta1.CloudManagementParameters{
						ServiceManagerSecretNamespace: "someNamespace",
						ServiceManagerSecret:          "someSecret",
					}),
					WithStatus(v1beta1.CloudManagementObservation{
						DataSourceLookup: &v1beta1.CloudManagementDataSourceLookup{
							CloudManagementPlanID: "planID",
						},
						Instance: &v1beta1.Instance{
							Id: internal.Ptr("someID"),
						},
						Binding: &v1beta1.Binding{
							Id: internal.Ptr("anotherID"),
						},
					}),
				),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &connector{
				kube:                  &tc.args.kube,
				usage:                 test2.NoOpReferenceResolverTracker{},
				resourcetracker:       test2.NoOpReferenceResolverTracker{},
				newClientInitalizerFn: tc.args.clientInitializerFn,
				newPlanIdResolverFn:   tc.args.planIdResolverFn,
			}
			_, err := uua.Connect(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(err, tc.want.err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.args.cr, tc.want.cr); diff != "" {
				t.Errorf("\ne.Observe(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestObserve(t *testing.T) {
	type want struct {
		err error
		obs managed.ExternalObservation
		cr  *v1beta1.CloudManagement
	}
	type args struct {
		cr       *v1beta1.CloudManagement
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
				cr: NewCloudManagement("test"),
				tfClient: &TfClientFake{
					observeFn: func() (cmclient.ResourcesStatus, error) {
						return cmclient.ResourcesStatus{}, errors.New("observeError")
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{},
				err: errors.New("observeError"),
				cr: NewCloudManagement("test",
					WithStatus(v1beta1.CloudManagementObservation{
						Status: v1alpha1.CisStatusUnbound,
					}),
					WithConditions(xpv1.Unavailable())),
			},
		},
		{
			name: "NotAvailable",
			args: args{
				cr: NewCloudManagement("test"),
				tfClient: &TfClientFake{
					observeFn: func() (cmclient.ResourcesStatus, error) {
						// Doesn't matter what observe is returned exactly, as long as its passed through and IDs are persisted
						return cmclient.ResourcesStatus{
							ExternalObservation: managed.ExternalObservation{ResourceExists: false},
							Instance:            v1alpha1.SubaccountServiceInstanceObservation{ID: internal.Ptr("someID")},
						}, nil
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{ResourceExists: false},
				err: nil,
				cr: NewCloudManagement("test",
					WithStatus(v1beta1.CloudManagementObservation{
						Status:            v1alpha1.CisStatusUnbound,
						ServiceInstanceID: "someID",
						Instance:          &v1beta1.Instance{Id: internal.Ptr("someID")},
					}),
					WithConditions(xpv1.Unavailable()),
				),
			},
		},
		{
			name: "IsAvailable",
			args: args{
				cr: NewCloudManagement("test"),
				tfClient: &TfClientFake{
					observeFn: func() (cmclient.ResourcesStatus, error) {
						// Doesn't matter if updated or not
						return cmclient.ResourcesStatus{
							ExternalObservation: managed.ExternalObservation{
								ResourceExists:    true,
								ResourceUpToDate:  true,
								ConnectionDetails: map[string][]byte{"key": []byte("value")},
							},
							Instance: v1alpha1.SubaccountServiceInstanceObservation{ID: internal.Ptr("someID")},
							Binding:  v1alpha1.SubaccountServiceBindingObservation{ID: internal.Ptr("anotherID")},
						}, nil

					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: map[string][]byte{"key": []byte("value")}},
				err: nil,
				cr: NewCloudManagement("test",
					WithStatus(v1beta1.CloudManagementObservation{
						Status:            v1alpha1.CisStatusBound,
						ServiceInstanceID: "someID",
						ServiceBindingID:  "anotherID",
						Instance:          &v1beta1.Instance{Id: internal.Ptr("someID")},
						Binding:           &v1beta1.Binding{Id: internal.Ptr("anotherID")},
					}),
					WithConditions(xpv1.Available())),
			},
		},
		{
			name: "IsAvailableWithContext",
			args: args{
				cr: NewCloudManagement("test"),
				tfClient: &TfClientFake{
					observeFn: func() (cmclient.ResourcesStatus, error) {
						// Doesn't matter if updated or not
						return cmclient.ResourcesStatus{
							ExternalObservation: managed.ExternalObservation{
								ResourceExists:    true,
								ResourceUpToDate:  true,
								ConnectionDetails: map[string][]byte{"key": []byte("value")},
							},
							Instance: v1alpha1.SubaccountServiceInstanceObservation{ID: internal.Ptr("someID"), Context: internal.Ptr(`{"key":"value"}`)},
							Binding:  v1alpha1.SubaccountServiceBindingObservation{ID: internal.Ptr("anotherID")},
						}, nil

					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: map[string][]byte{"key": []byte("value")}},
				err: nil,
				cr: NewCloudManagement("test",
					WithStatus(v1beta1.CloudManagementObservation{
						Status:            v1alpha1.CisStatusBound,
						ServiceInstanceID: "someID",
						ServiceBindingID:  "anotherID",
						Instance:          &v1beta1.Instance{Id: internal.Ptr("someID"), Context: &map[string]string{"key": "value"}},
						Binding:           &v1beta1.Binding{Id: internal.Ptr("anotherID")},
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
		cr  *v1beta1.CloudManagement
	}
	type args struct {
		cr       *v1beta1.CloudManagement
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
				cr: NewCloudManagement("test"),
				tfClient: &TfClientFake{
					createFn: func() (string, string, error) {
						return "", "", errors.New("createError")
					},
				},
			},
			want: want{
				err: errors.New("createError"),
				cr:  NewCloudManagement("test", WithConditions(xpv1.Creating())),
			},
		},
		{
			name: "Success",
			args: args{
				cr: NewCloudManagement("test"),
				tfClient: &TfClientFake{
					createFn: func() (string, string, error) {
						return "someID", "anotherID", nil
					},
				},
			},
			want: want{
				err: nil,
				cr: NewCloudManagement("test",
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

func TestDelete(t *testing.T) {
	type want struct {
		err error
		cr  *v1beta1.CloudManagement
	}
	type args struct {
		cr       *v1beta1.CloudManagement
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
				cr: NewCloudManagement("test", WithExternalName("someID/anotherID")),
				tfClient: &TfClientFake{
					deleteFn: func() error {
						return errors.New("deleteError")
					},
				},
			},
			want: want{
				err: errors.New("deleteError"),
				cr:  NewCloudManagement("test", WithExternalName("someID/anotherID"), WithConditions(xpv1.Deleting())),
			},
		},
		{
			name: "Success",
			args: args{
				cr: NewCloudManagement("test", WithExternalName("someID/anotherID")),
				tfClient: &TfClientFake{
					deleteFn: func() error {
						return nil
					},
				},
			},
			want: want{
				err: nil,
				cr:  NewCloudManagement("test", WithExternalName("someID/anotherID"), WithConditions(xpv1.Deleting())),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &external{
				tracker:  test2.NoOpReferenceResolverTracker{},
				tfClient: tc.args.tfClient,
			}
			err := uua.Delete(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Delete(): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\ne.Delete(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type want struct {
		err error
	}
	type args struct {
		tfClient *TfClientFake
	}
	tests := []struct {
		name string
		args args

		want want
	}{
		{
			name: "Update Error",
			args: args{
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
				tracker:  test2.NoOpReferenceResolverTracker{},
				tfClient: tc.args.tfClient,
			}
			_, err := uua.Update(context.TODO(), NewCloudManagement("test", WithExternalName("someID/anotherID")))
			if diff := cmp.Diff(err, tc.want.err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Update(): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}

// Utils
func NewCloudManagement(name string, m ...CloudManagementModifier) *v1beta1.CloudManagement {
	cr := &v1beta1.CloudManagement{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	meta.SetExternalName(cr, name)
	for _, f := range m {
		f(cr)
	}
	return cr
}

// this pattern can be potentially auto generated, its quite useful to write expressive unittests
type CloudManagementModifier func(dirEnvironment *v1beta1.CloudManagement)

func WithStatus(status v1beta1.CloudManagementObservation) CloudManagementModifier {
	return func(r *v1beta1.CloudManagement) {
		r.Status.AtProvider = status
	}
}

func WithData(data v1beta1.CloudManagementParameters) CloudManagementModifier {
	return func(r *v1beta1.CloudManagement) {
		r.Spec.ForProvider = data
	}
}

func WithConditions(c ...xpv1.Condition) CloudManagementModifier {
	return func(r *v1beta1.CloudManagement) { r.Status.ConditionedStatus.Conditions = c }
}

func WithExternalName(externalName string) CloudManagementModifier {
	return func(r *v1beta1.CloudManagement) {
		meta.SetExternalName(r, externalName)
	}
}

// Fakes

var _ cmclient.ITfClient = &TfClientFake{}

type TfClientFake struct {
	observeFn func() (cmclient.ResourcesStatus, error)
	createFn  func() (string, string, error)
	updateFn  func() error
	deleteFn  func() error
}

func (t TfClientFake) ObserveResources(ctx context.Context, cr *v1beta1.CloudManagement) (cmclient.ResourcesStatus, error) {
	return t.observeFn()
}

func (t TfClientFake) CreateResources(ctx context.Context, cr *v1beta1.CloudManagement) (string, string, error) {
	return t.createFn()
}

func (t TfClientFake) UpdateResources(ctx context.Context, cr *v1beta1.CloudManagement) error {
	return t.updateFn()
}

func (t TfClientFake) DeleteResources(ctx context.Context, cr *v1beta1.CloudManagement) error {
	return t.deleteFn()
}

var _ servicemanager.PlanIdResolver = &PlanIDFake{}

type PlanIDFake struct {
	PlanIDByNameFn func(ctx context.Context, offeringName string, servicePlanName string) (string, error)
}

func (p PlanIDFake) PlanIDByName(ctx context.Context, offeringName string, servicePlanName string) (string, error) {
	return p.PlanIDByNameFn(ctx, offeringName, servicePlanName)
}

var _ cmclient.ITfClientInitializer = &ClientInitializerFake{}

type ClientInitializerFake struct {
	ConnectResourcesFn func(ctx context.Context, cr *v1beta1.CloudManagement) (cmclient.ITfClient, error)
}

func (c ClientInitializerFake) ConnectResources(ctx context.Context, cr *v1beta1.CloudManagement) (cmclient.ITfClient, error) {
	return c.ConnectResourcesFn(ctx, cr)
}
