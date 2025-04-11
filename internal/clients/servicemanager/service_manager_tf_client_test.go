package servicemanager

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	"github.com/sap/crossplane-provider-btp/internal"
)

const DefaultServiceName = "test-default-name"
const DefaultBindingName = "test-default-binding"

func TestConnectResources(t *testing.T) {
	type want struct {
		err          error
		subaccountId string
		planId       string
		instanceSpec v1alpha1.SubaccountServiceInstanceParameters
		bindingSpec  v1alpha1.SubaccountServiceBindingParameters
	}
	tests := []struct {
		name string
		cr   *v1beta1.ServiceManager

		instanceConnectorMock func() (managed.ExternalClient, error)
		bindingConnectorMock  func() (managed.ExternalClient, error)

		want want
	}{
		{
			name: "InstanceError",
			cr:   testSMCr("subaccountId", "planId", "", "", "", ""),
			instanceConnectorMock: func() (managed.ExternalClient, error) {
				return nil, errors.New("instanceConnectError")
			},
			want: want{
				err: errors.New("instanceConnectError"),
			},
		},
		{
			name: "BindingError",
			cr:   testSMCr("subaccountId", "planId", "", "", "", ""),
			instanceConnectorMock: func() (managed.ExternalClient, error) {
				return ExternalClientFake{}, nil
			},
			bindingConnectorMock: func() (managed.ExternalClient, error) {
				return nil, errors.New("bindingConnectError")
			},
			want: want{
				err: errors.New("bindingConnectError"),
			},
		},
		{
			name: "SuccessWithDefaults",
			cr:   testSMCr("subaccountId", "planId", "instanceID", "instanceID", "", ""),
			instanceConnectorMock: func() (managed.ExternalClient, error) {
				return ExternalClientFake{}, nil
			},
			bindingConnectorMock: func() (managed.ExternalClient, error) {
				return ExternalClientFake{}, nil
			},
			want: want{
				subaccountId: "subaccountId",
				planId:       "planId",
				instanceSpec: v1alpha1.SubaccountServiceInstanceParameters{
					Name:          internal.Ptr(DefaultServiceName),
					ServiceplanID: internal.Ptr("planId"),
					SubaccountID:  internal.Ptr("subaccountId"),
				},
				bindingSpec: v1alpha1.SubaccountServiceBindingParameters{
					SubaccountID:      internal.Ptr("subaccountId"),
					Name:              internal.Ptr(DefaultBindingName),
					ServiceInstanceID: internal.Ptr("instanceID"),
				},
			},
		},
		{
			name: "SuccessWithUserData",
			cr:   testSMCr("subaccountId", "planId", "instanceID", "instanceID", "custom-name", "another-custom-name"),
			instanceConnectorMock: func() (managed.ExternalClient, error) {
				return ExternalClientFake{}, nil
			},
			bindingConnectorMock: func() (managed.ExternalClient, error) {
				return ExternalClientFake{}, nil
			},
			want: want{
				subaccountId: "subaccountId",
				planId:       "planId",
				instanceSpec: v1alpha1.SubaccountServiceInstanceParameters{
					Name:          internal.Ptr("custom-name"),
					ServiceplanID: internal.Ptr("planId"),
					SubaccountID:  internal.Ptr("subaccountId"),
				},
				bindingSpec: v1alpha1.SubaccountServiceBindingParameters{
					SubaccountID:      internal.Ptr("subaccountId"),
					Name:              internal.Ptr("another-custom-name"),
					ServiceInstanceID: internal.Ptr("instanceID"),
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resources, err := NewServiceManagerTfClient(
				&ExternalConnectorFake{
					tc.instanceConnectorMock,
				},
				&ExternalConnectorFake{
					tc.bindingConnectorMock,
				},

				Defaults{DefaultServiceName, DefaultBindingName},
			).ConnectResources(context.TODO(), tc.cr)
			if diff := cmp.Diff(err, tc.want.err, test.EquateErrors()); diff != "" {
				t.Errorf("ConnectResources() got unexpected error result %v, want %v", err, tc.want.err)
			}
			if tc.want.err == nil {
				if resources == nil {
					t.Errorf("ConnectResources() didn't return a result, but its expected")
				}
				tfClient := resources.(*TfClient)
				if diff := cmp.Diff(tc.want.instanceSpec, tfClient.sInstance.Spec.ForProvider); diff != "" {
					t.Errorf("\ne.ConnectResources() instance spec: -want, +got:\n%s\n", diff)
				}
				if diff := cmp.Diff(tc.want.bindingSpec, tfClient.sBinding.Spec.ForProvider); diff != "" {
					t.Errorf("\ne.ConnectResources() binding spec: -want, +got:\n%s\n", diff)
				}
			}
		})
	}
}

func TestObserveResources(t *testing.T) {
	type want struct {
		err error
		obs ResourcesStatus
	}
	type args struct {
		cr *v1beta1.ServiceManager

		siExternal ExternalClientFake
		sbExternal ExternalClientFake

		sInstance *v1alpha1.SubaccountServiceInstance
		sBinding  *v1alpha1.SubaccountServiceBinding
	}
	tests := []struct {
		name string
		args args

		want want
	}{
		{
			name: "InstanceObserveError",
			args: args{
				cr: testSMCr("subaccountId", "planId", "someID", "", "", ""),
				siExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{}, errors.New("instanceObserveError")
					},
				},
				sbExternal: ExternalClientFake{},
				sInstance:  testServiceInstance("someID"),
				sBinding:   testServiceBinding("someCr"),
			},
			want: want{
				err: errors.New("instanceObserveError"),
			},
		},
		{
			name: "InstanceNeedsCreation",
			args: args{
				cr: testSMCr("subaccountId", "planId", "someID", "", "", ""),
				siExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: false}, nil
					},
				},
				sbExternal: ExternalClientFake{},
				sInstance:  testServiceInstance("someID"),
				sBinding:   testServiceBinding("someCr"),
			},
			want: want{
				obs: ResourcesStatus{
					ExternalObservation: managed.ExternalObservation{ResourceExists: false},
				},
			},
		},
		{
			name: "BindingObserveError",
			args: args{
				cr: testSMCr("subaccountId", "planId", "someID/anotherID", "", "", ""),
				siExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
					},
				},
				sbExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{}, errors.New("bindingObserveError")
					},
				},
				sInstance: testServiceInstance("someID"),
				sBinding:  testServiceBinding("anotherID"),
			},
			want: want{
				err: errors.New("bindingObserveError"),
			},
		},
		{
			name: "BindingNeedsCreation",
			args: args{
				cr: testSMCr("subaccountId", "planId", "someID", "", "", ""),
				siExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
					},
				},
				sbExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: false}, nil
					},
				},
				sInstance: testServiceInstance("someID"),
				sBinding:  testServiceBinding("anotherID"),
			},
			want: want{
				obs: ResourcesStatus{
					ExternalObservation: managed.ExternalObservation{ResourceExists: false},
					InstanceID:          "someID",
				},
				err: nil,
			},
		},
		{
			name: "InstanceNeedsUpdate",
			args: args{
				cr: testSMCr("subaccountId", "planId", "someID/anotherID", "", "", ""),
				siExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, nil
					},
				},
				sbExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true,
							ConnectionDetails: map[string][]byte{"attribute.credentials": []byte(`{"clientid":"someClientID","clientsecret":"someSecret","sm_url":"https://service-manager.cfapps.eu12.hana.ondemand.com","url":"https://subdomain.authentication.eu12.hana.ondemand.com","xsappname":"someAppName"}`)}}, nil
					},
				},
				sInstance: testServiceInstance("someID"),
				sBinding:  testServiceBinding("anotherID"),
			},
			want: want{
				obs: ResourcesStatus{
					ExternalObservation: managed.ExternalObservation{
						ResourceExists:   true,
						ResourceUpToDate: false,
						ConnectionDetails: map[string][]byte{
							v1beta1.ResourceCredentialsClientSecret:      []byte("someSecret"),
							v1beta1.ResourceCredentialsClientId:          []byte("someClientID"),
							v1beta1.ResourceCredentialsServiceManagerUrl: []byte("https://service-manager.cfapps.eu12.hana.ondemand.com"),
							v1beta1.ResourceCredentialsXsuaaUrl:          []byte("https://subdomain.authentication.eu12.hana.ondemand.com"),
							v1beta1.ResourceCredentialsXsappname:         []byte("someAppName"),
							v1beta1.ResourceCredentialsXsuaaUrlSufix:     []byte("/oauth/token"),
						},
					},
					InstanceID: "someID",
					BindingID:  "anotherID",
				},
				err: nil,
			},
		},
		{
			// in case of missing binding and changed instance we expect to first create the binding and later update the instance in a second reconcilation loop
			name: "CreationPrecedesUpdate",
			args: args{
				cr: testSMCr("subaccountId", "planId", "someID/anotherID", "", "", ""),
				siExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, nil
					},
				},
				sbExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: false}, nil
					},
				},
				sInstance: testServiceInstance("someID"),
				sBinding:  testServiceBinding("anotherID"),
			},
			want: want{
				obs: ResourcesStatus{
					ExternalObservation: managed.ExternalObservation{ResourceExists: false},
					InstanceID:          "someID",
				},
				err: nil,
			},
		},
		{
			name: "UnexpectedFormatOfReturnedConnectionDetails",
			args: args{
				cr: testSMCr("subaccountId", "planId", "someID/anotherID", "", "", ""),
				siExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, nil
					},
				},
				sbExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						// return no connection details isn't expected in this case
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
					},
				},
				sInstance: testServiceInstance("someID"),
				sBinding:  testServiceBinding("anotherID"),
			},
			want: want{
				err: errors.Wrap(errors.New("unexpected end of JSON input"), "Unexpected format of returned connectionDetails"),
			},
		},
		{
			name: "AllResourcesSynced",
			args: args{
				cr: testSMCr("subaccountId", "planId", "someID/anotherID", "", "", ""),
				siExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
					},
				},
				sbExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true,
							ConnectionDetails: map[string][]byte{"attribute.credentials": []byte(`{"clientid":"someClientID","clientsecret":"someSecret","sm_url":"https://service-manager.cfapps.eu12.hana.ondemand.com","url":"https://subdomain.authentication.eu12.hana.ondemand.com","xsappname":"someAppName"}`)},
						}, nil
					},
				},
				sInstance: testServiceInstance("someID"),
				sBinding:  testServiceBinding("anotherID"),
			},
			want: want{
				obs: ResourcesStatus{
					ExternalObservation: managed.ExternalObservation{
						ResourceExists:   true,
						ResourceUpToDate: true,
						ConnectionDetails: map[string][]byte{
							v1beta1.ResourceCredentialsClientSecret:      []byte("someSecret"),
							v1beta1.ResourceCredentialsClientId:          []byte("someClientID"),
							v1beta1.ResourceCredentialsServiceManagerUrl: []byte("https://service-manager.cfapps.eu12.hana.ondemand.com"),
							v1beta1.ResourceCredentialsXsuaaUrl:          []byte("https://subdomain.authentication.eu12.hana.ondemand.com"),
							v1beta1.ResourceCredentialsXsappname:         []byte("someAppName"),
							v1beta1.ResourceCredentialsXsuaaUrlSufix:     []byte("/oauth/token"),
						}},
					InstanceID: "someID",
					BindingID:  "anotherID",
				},
				err: nil,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &TfClient{
				siExternal: tc.args.siExternal,
				sbExternal: tc.args.sbExternal,
				sInstance:  tc.args.sInstance,
				sBinding:   tc.args.sBinding,
			}
			obs, err := uua.ObserveResources(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(tc.want.obs, obs); diff != "" {
				t.Errorf("\ne.ObserveResources(): -want, +got:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.ObserveResources(): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}

func TestCreateResources(t *testing.T) {
	type want struct {
		err error
		sID string
		bID string
	}
	type args struct {
		cr *v1beta1.ServiceManager

		siExternal ExternalClientFake
		sbExternal ExternalClientFake

		sInstance *v1alpha1.SubaccountServiceInstance
		sBinding  *v1alpha1.SubaccountServiceBinding
	}
	tests := []struct {
		name string
		args args

		want want
	}{
		{
			name: "InstanceCreateError",
			args: args{
				cr: testSMCr("subaccountId", "planId", "crName", "", "", ""),
				siExternal: ExternalClientFake{
					createFn: func(mg resource.Managed) (managed.ExternalCreation, error) {
						return managed.ExternalCreation{}, errors.New("instanceCreateError")
					},
				},
				sInstance: testServiceInstance("crName"),
			},
			want: want{
				err: errors.New("instanceCreateError"),
			},
		},
		{
			name: "InstanceCreateSuccess",
			args: args{
				cr: testSMCr("subaccountId", "planId", "crName", "", "", ""),
				siExternal: ExternalClientFake{
					createFn: func(mg resource.Managed) (managed.ExternalCreation, error) {
						meta.SetExternalName(mg, "someID")
						return managed.ExternalCreation{}, nil
					},
				},
				sInstance: testServiceInstance("crName"),
			},
			want: want{
				sID: "someID",
			},
		},
		{
			// we should return an error also even if only the binding creation fails
			name: "BindingCreateError",
			args: args{
				cr: testSMCr("subaccountId", "planId", "crName", "someID", "", ""),
				sbExternal: ExternalClientFake{
					createFn: func(mg resource.Managed) (managed.ExternalCreation, error) {
						return managed.ExternalCreation{}, errors.New("bindingCreateError")
					},
				},
				sInstance: testServiceInstance("someID"),
				sBinding:  testServiceBinding("crName"),
			},
			want: want{
				err: errors.New("bindingCreateError"),
				sID: "someID",
				bID: "",
			},
		},
		{
			name: "BindingCreateSuccess",
			args: args{
				cr: testSMCr("subaccountId", "planId", "crName", "someID", "", ""),
				siExternal: ExternalClientFake{
					createFn: func(mg resource.Managed) (managed.ExternalCreation, error) {
						setExternalName(mg, "someID")
						return managed.ExternalCreation{}, nil
					},
				},
				sbExternal: ExternalClientFake{
					createFn: func(mg resource.Managed) (managed.ExternalCreation, error) {
						setExternalName(mg, "anotherID")
						return managed.ExternalCreation{}, nil
					},
				},
				sInstance: testServiceInstance("someID"),
				sBinding:  testServiceBinding("crName"),
			},
			want: want{
				sID: "someID",
				bID: "anotherID",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &TfClient{
				siExternal: tc.args.siExternal,
				sbExternal: tc.args.sbExternal,
				sInstance:  tc.args.sInstance,
				sBinding:   tc.args.sBinding,
			}
			sID, bID, err := uua.CreateResources(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(tc.want.sID, sID); diff != "" {
				t.Errorf("\ne.CreateResources(): -want, +got:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.bID, bID); diff != "" {
				t.Errorf("\ne.CreateResources(): -want, +got:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.CreateResources(): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}

func TestUpdateResources(t *testing.T) {
	type want struct {
		err error
	}
	type args struct {
		cr *v1beta1.ServiceManager

		siExternal ExternalClientFake
		sbExternal ExternalClientFake

		sInstance *v1alpha1.SubaccountServiceInstance
		sBinding  *v1alpha1.SubaccountServiceBinding
	}
	tests := []struct {
		name string
		args args

		want want
	}{
		{
			name: "InstanceUpdateError",
			args: args{
				cr: testSMCr("subaccountId", "planId", "someID/anotherID", "someID", "changedName", ""),
				siExternal: ExternalClientFake{
					updateFn: func() (managed.ExternalUpdate, error) {
						return managed.ExternalUpdate{}, errors.New("instanceUpdateError")
					},
				},
				sInstance: testServiceInstance("someID"),
			},
			want: want{
				err: errors.New("instanceUpdateError"),
			},
		},
		{
			name: "InstanceUpdateSuccess",
			args: args{
				cr: testSMCr("subaccountId", "planId", "someID/anotherID", "someUD", "changedName", ""),
				siExternal: ExternalClientFake{
					updateFn: func() (managed.ExternalUpdate, error) {
						return managed.ExternalUpdate{}, nil
					},
				},
				sInstance: testServiceInstance("someID"),
			},
			want: want{
				err: nil,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uua := &TfClient{
				siExternal: tc.args.siExternal,
				sbExternal: tc.args.sbExternal,
				sInstance:  tc.args.sInstance,
				sBinding:   tc.args.sBinding,
			}
			err := uua.UpdateResources(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(err, tc.want.err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.UpdateResources(): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}

func TestDeleteResources(t *testing.T) {
	type want struct {
		err error
	}
	type args struct {
		cr *v1beta1.ServiceManager

		siExternal ExternalClientFake
		sbExternal ExternalClientFake
	}
	tests := []struct {
		name string
		args args

		want want
	}{
		{
			name: "BindingDeleteError",
			args: args{
				cr: testSMCr("subaccountId", "planId", "someID/anotherID", "", "", ""),
				sbExternal: ExternalClientFake{
					deleteFn: func() (managed.ExternalDelete, error) {
						return managed.ExternalDelete{}, errors.New("bindingDeleteError")
					},
				},
			},
			want: want{
				err: errors.New("bindingDeleteError"),
			},
		},
		{
			name: "InstanceDeleteError",
			args: args{
				cr: testSMCr("subaccountId", "planId", "someID/anotherID", "", "", ""),
				sbExternal: ExternalClientFake{
					deleteFn: func() (managed.ExternalDelete, error) {
						return managed.ExternalDelete{}, nil
					},
				},
				siExternal: ExternalClientFake{
					deleteFn: func() (managed.ExternalDelete, error) {
						return managed.ExternalDelete{}, errors.New("instanceDeleteError")
					},
				},
			},
			want: want{
				err: errors.New("instanceDeleteError"),
			},
		},
		{
			name: "DeleteSuccess",
			args: args{
				cr: testSMCr("subaccountId", "planId", "someID/anotherID", "", "", ""),
				sbExternal: ExternalClientFake{
					deleteFn: func() (managed.ExternalDelete, error) {
						return managed.ExternalDelete{}, nil
					},
				},
				siExternal: ExternalClientFake{
					deleteFn: func() (managed.ExternalDelete, error) {
						return managed.ExternalDelete{}, nil
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
			uua := &TfClient{
				siExternal: tc.args.siExternal,
				sbExternal: tc.args.sbExternal,
			}
			err := uua.DeleteResources(context.TODO(), tc.args.cr)
			if diff := cmp.Diff(err, tc.want.err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.DeleteResources(): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}

// Utils
func testSMCr(saId, planId, extName, statusInstanceID, serviceInstanceName, serviceBindingName string) *v1beta1.ServiceManager {
	sm := &v1beta1.ServiceManager{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: v1beta1.ServiceManagerSpec{
			ForProvider: v1beta1.ServiceManagerParameters{
				SubaccountGuid:      saId,
				ServiceInstanceName: serviceInstanceName,
				ServiceBindingName:  serviceBindingName,
			},
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: "default",
				},
			},
		},
		Status: v1beta1.ServiceManagerStatus{
			AtProvider: v1beta1.ServiceManagerObservation{
				DataSourceLookup: &v1beta1.DataSourceLookup{
					ServiceManagerPlanID: planId,
				},
				ServiceInstanceID: statusInstanceID,
			},
		},
	}
	meta.SetExternalName(sm, extName)
	return sm
}

func testServiceInstance(extName string) *v1alpha1.SubaccountServiceInstance {

	instance := &v1alpha1.SubaccountServiceInstance{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{},
		},
		Spec:   v1alpha1.SubaccountServiceInstanceSpec{},
		Status: v1alpha1.SubaccountServiceInstanceStatus{},
	}
	meta.SetExternalName(instance, extName)
	return instance
}

func testServiceBinding(extName string) *v1alpha1.SubaccountServiceBinding {
	binding := &v1alpha1.SubaccountServiceBinding{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       v1alpha1.SubaccountServiceBindingSpec{},
		Status:     v1alpha1.SubaccountServiceBindingStatus{},
	}
	meta.SetExternalName(binding, extName)
	return binding
}

// Fakes
// Fake connectors from the embedded instance and binding resources / using whole tf roundtrip here would require external connection
var _ managed.ExternalConnecter = &ExternalConnectorFake{}

type ExternalConnectorFake struct {
	connectFn func() (managed.ExternalClient, error)
}

func (e ExternalConnectorFake) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	return e.connectFn()
}

// Fake controllers returned from embedded instance and binding connectors
var _ managed.ExternalClient = &ExternalClientFake{}

type ExternalClientFake struct {
	observeFn func() (managed.ExternalObservation, error)
	createFn  func(mg resource.Managed) (managed.ExternalCreation, error)
	updateFn  func() (managed.ExternalUpdate, error)
	deleteFn  func() (managed.ExternalDelete, error)
}

func (c ExternalClientFake) Disconnect(ctx context.Context) error {
	// No-op
	return nil
}
func (e ExternalClientFake) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	return e.observeFn()
}

func (e ExternalClientFake) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	return e.createFn(mg)
}

func (e ExternalClientFake) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	return e.updateFn()
}

func (e ExternalClientFake) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	return e.deleteFn()
}

func setExternalName(mg resource.Managed, name string) {
	meta.SetExternalName(mg, name)
}
