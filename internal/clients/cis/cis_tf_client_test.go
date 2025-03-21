package cis

import (
	"context"
	"encoding/json"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	bindingData string = `{"endpoints":{"accounts_service_url":"xxx","cloud_automation_url":"xxx","entitlements_service_url":"xxx","events_service_url":"xxx","metadata_service_url":"xxx","order_processing_url":"xxx","provisioning_service_url":"xxx","saas_registry_service_url":"xxx"},"grant_type":"client_credentials","sap.cloud.service":"com.sap.core.commercial.service.local","uaa":{"apiurl":"xxx","clientid":"xxx","clientsecret":"xxx","credential-type":"binding-secret","identityzone":"xxx","identityzoneid":"xxx","sburl":"xxx","serviceInstanceId":"xxx","subaccountid":"xxx","tenantid":"xxx","tenantmode":"shared","uaadomain":"xxx","url":"xxx","verificationkey":"xxx","xsappname":"xxx","xsmasterappname":"xxx","zoneid":"xxx"}}`

	defaultInstanceName     string = "default-si-name"
	defaultBindingName      string = "default-binding-name"
	defaultName             string = "test"
	defaultSaId             string = "defaultSaId"
	defaultPlanId           string = "defaultPlanId"
	defaultExtName          string = "defaultExtName"
	defaultStatusInstanceID string = "defaultStatusInstanceID"

	defaultInstanceID string = "someID"
	defaultBindingID  string = "anotherID"
)

var defaultCR = testCMCr(utilCloudManagementParams{extName: defaultExtName, siName: defaultInstanceName, sbName: defaultBindingName, statusInstanceID: defaultStatusInstanceID})

func TestConnectResources(t *testing.T) {
	successInstanceMock := func() (managed.ExternalClient, error) {
		return ExternalClientFake{}, nil
	}
	successBindingMock := func() (managed.ExternalClient, error) {
		return ExternalClientFake{}, nil
	}
	errorBindingMock := func() (managed.ExternalClient, error) {
		return nil, errors.New("bindingConnectError")
	}

	getExpectedInstanceSpec := func(name string) v1alpha1.SubaccountServiceInstanceParameters {
		return v1alpha1.SubaccountServiceInstanceParameters{
			Name:          internal.Ptr(name),
			ServiceplanID: internal.Ptr(defaultPlanId),
			SubaccountID:  internal.Ptr(defaultSaId),
			Parameters:    internal.Ptr(`{"grantType":"clientCredentials"}`),
		}
	}

	getExpectedBindingSpec := func(name, extName string) v1alpha1.SubaccountServiceBindingParameters {
		return v1alpha1.SubaccountServiceBindingParameters{
			SubaccountID:      internal.Ptr(defaultSaId),
			Name:              internal.Ptr(name),
			ServiceInstanceID: internal.Ptr(extName),
		}
	}

	tests := []struct {
		name              string
		cr                *v1beta1.CloudManagement
		instanceConnector func() (managed.ExternalClient, error)
		bindingConnector  func() (managed.ExternalClient, error)
		wantErr           error
		expectedInstance  v1alpha1.SubaccountServiceInstanceParameters
		expectedBinding   v1alpha1.SubaccountServiceBindingParameters
	}{
		{
			name:              "BindingError",
			cr:                defaultCR,
			instanceConnector: successInstanceMock,
			bindingConnector:  errorBindingMock,
			wantErr:           errors.New("bindingConnectError"),
		},
		{
			name:              "Success",
			cr:                defaultCR,
			instanceConnector: successInstanceMock,
			bindingConnector:  successBindingMock,
			expectedInstance:  getExpectedInstanceSpec(defaultInstanceName),
			expectedBinding:   getExpectedBindingSpec(defaultBindingName, defaultExtName),
		},
		{
			name:              "Success with default instance name",
			cr:                testCMCr(utilCloudManagementParams{extName: defaultExtName, sbName: defaultBindingName}),
			instanceConnector: successInstanceMock,
			bindingConnector:  successBindingMock,
			expectedInstance:  getExpectedInstanceSpec(v1beta1.DefaultCloudManagementInstanceName),
			expectedBinding:   getExpectedBindingSpec(defaultBindingName, defaultExtName),
		},
		{
			name:              "Success with default binding name",
			cr:                testCMCr(utilCloudManagementParams{extName: defaultExtName, siName: defaultInstanceName}),
			instanceConnector: successInstanceMock,
			bindingConnector:  successBindingMock,
			expectedInstance:  getExpectedInstanceSpec(defaultInstanceName),
			expectedBinding:   getExpectedBindingSpec(v1beta1.DefaultCloudManagementBindingName, defaultExtName),
		},
		{
			name:              "Success with default instance and binding name",
			cr:                testCMCr(utilCloudManagementParams{extName: defaultExtName}),
			instanceConnector: successInstanceMock,
			bindingConnector:  successBindingMock,
			expectedInstance:  getExpectedInstanceSpec(v1beta1.DefaultCloudManagementInstanceName),
			expectedBinding:   getExpectedBindingSpec(v1beta1.DefaultCloudManagementBindingName, defaultExtName),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resources, err := NewTfClient(
				&ExternalConnectorFake{tc.instanceConnector},
				&ExternalConnectorFake{tc.bindingConnector},
			).ConnectResources(context.TODO(), tc.cr)

			if diff := cmp.Diff(err, tc.wantErr, test.EquateErrors()); diff != "" {
				t.Errorf("ConnectResources() error mismatch (-want +got):\n%s", diff)
			}

			if tc.wantErr == nil {
				if resources == nil {
					t.Errorf("ConnectResources() returned nil, expected a result")
				}
				tfClient := resources.(*TfClient)

				if diff := cmp.Diff(tc.expectedInstance, tfClient.sInstance.Spec.ForProvider); diff != "" {
					t.Errorf("Instance spec mismatch (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tc.expectedBinding, tfClient.sBinding.Spec.ForProvider); diff != "" {
					t.Errorf("Binding spec mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestObserveResources(t *testing.T) {
	var expectedConversion = func(bindingData string) map[string][]byte {
		var creds map[string]interface{}
		err := json.Unmarshal([]byte(bindingData), &creds)
		if err != nil {
			t.Errorf("Error unmarshalling bindingData: %v", err)
		}
		credentials := internal.Flatten(creds)
		raw, _ := json.Marshal(creds)
		credentials[providerv1alpha1.RawBindingKey] = raw
		return credentials
	}

	type want struct {
		err error
		obs ResourcesStatus
	}
	type args struct {
		siExternal ExternalClientFake
		sbExternal ExternalClientFake
		siName     string
		cr         *v1beta1.CloudManagement
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "InstanceObserveError",
			args: args{
				siExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{}, errors.New("instanceObserveError")
					},
				},
				sbExternal: ExternalClientFake{},
			},
			want: want{
				err: errors.New("instanceObserveError"),
			},
		},
		{
			name: "InstanceNeedsCreation",
			args: args{
				siExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: false}, nil
					},
				},
				sbExternal: ExternalClientFake{},
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
			},
			want: want{
				err: errors.New("bindingObserveError"),
			},
		},
		{
			name: "BindingNeedsCreation",
			args: args{
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
			},
			want: want{
				obs: ResourcesStatus{
					ExternalObservation: managed.ExternalObservation{ResourceExists: false},
					Instance:            v1alpha1.SubaccountServiceInstanceObservation{ID: internal.Ptr(defaultInstanceID), Name: internal.Ptr(defaultInstanceName)},
				},
				err: nil,
			},
		},
		{
			// in case of missing binding and changed instance we expect to first create the binding and later update the instance in a second reconcilation loop
			name: "CreationPrecedesUpdate",
			args: args{
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
			},
			want: want{
				obs: ResourcesStatus{
					ExternalObservation: managed.ExternalObservation{ResourceExists: false},
					Instance:            v1alpha1.SubaccountServiceInstanceObservation{ID: internal.Ptr(defaultInstanceID), Name: internal.Ptr(defaultInstanceName)},
				},
				err: nil,
			},
		},
		{
			name: "DontUpdateOnV1Alpha1",
			args: args{
				siExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, nil
					},
				},
				sbExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true,
							ConnectionDetails: map[string][]byte{"attribute.credentials": []byte(bindingData)},
						}, nil
					},
				},
				siName: "test",
				cr:     testCMCr(utilCloudManagementParams{extName: defaultExtName, siName: "", crName: "test"}),
			},
			want: want{
				obs: ResourcesStatus{
					ExternalObservation: managed.ExternalObservation{
						ResourceExists:    true,
						ResourceUpToDate:  true,
						ConnectionDetails: expectedConversion(bindingData),
					},
					Instance: v1alpha1.SubaccountServiceInstanceObservation{ID: internal.Ptr(defaultInstanceID), Name: internal.Ptr("test")},
					Binding:  v1alpha1.SubaccountServiceBindingObservation{ID: internal.Ptr(defaultBindingID), Name: internal.Ptr(defaultBindingName)},
				},
				err: nil,
			},
		},
		{
			name: "UpdateOnV1Beta1",
			args: args{
				siExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, nil
					},
				},
				sbExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true,
							ConnectionDetails: map[string][]byte{"attribute.credentials": []byte(bindingData)},
						}, nil
					},
				},
				siName: "test",
				cr:     testCMCr(utilCloudManagementParams{extName: defaultExtName, siName: defaultInstanceName, crName: "test"}),
			},
			want: want{
				obs: ResourcesStatus{
					ExternalObservation: managed.ExternalObservation{
						ResourceExists:    true,
						ResourceUpToDate:  false,
						ConnectionDetails: expectedConversion(bindingData),
					},
					Instance: v1alpha1.SubaccountServiceInstanceObservation{ID: internal.Ptr(defaultInstanceID), Name: internal.Ptr("test")},
					Binding:  v1alpha1.SubaccountServiceBindingObservation{ID: internal.Ptr(defaultBindingID), Name: internal.Ptr(defaultBindingName)},
				},
				err: nil,
			},
		},
		{
			name: "UnexpectedFormatOfReturnedConnectionDetails",
			args: args{
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
			},
			want: want{
				err: errors.Wrap(errors.New("unexpected end of JSON input"), "Unexpected format of returned connectionDetails"),
			},
		},
		{
			name: "AllResourcesSynced",
			args: args{
				siExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
					},
				},
				sbExternal: ExternalClientFake{
					observeFn: func() (managed.ExternalObservation, error) {
						return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true,
							ConnectionDetails: map[string][]byte{"attribute.credentials": []byte(bindingData)},
						}, nil
					},
				},
			},
			want: want{
				obs: ResourcesStatus{
					ExternalObservation: managed.ExternalObservation{
						ResourceExists:    true,
						ResourceUpToDate:  true,
						ConnectionDetails: expectedConversion(bindingData),
					},
					Instance: v1alpha1.SubaccountServiceInstanceObservation{ID: internal.Ptr(defaultInstanceID), Name: internal.Ptr(defaultInstanceName)},
					Binding:  v1alpha1.SubaccountServiceBindingObservation{ID: internal.Ptr(defaultBindingID), Name: internal.Ptr(defaultBindingName)},
				},
				err: nil,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			siName := tc.args.siName
			if siName == "" {
				siName = defaultInstanceName
			}
			cr := tc.args.cr
			if cr == nil {
				cr = defaultCR
			}

			uua := &TfClient{
				siExternal: tc.args.siExternal,
				sbExternal: tc.args.sbExternal,
				sInstance:  testServiceInstance(defaultInstanceID, siName),
				sBinding:   testServiceBinding(defaultBindingID, defaultBindingName),
			}
			obs, err := uua.ObserveResources(context.TODO(), cr)
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
		cr *v1beta1.CloudManagement

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
				cr: testCMCr(utilCloudManagementParams{statusInstanceID: ""}),
				siExternal: ExternalClientFake{
					createFn: func(mg resource.Managed) (managed.ExternalCreation, error) {
						return managed.ExternalCreation{}, errors.New("instanceCreateError")
					},
				},
				sInstance: testServiceInstance(defaultInstanceID, defaultInstanceName),
			},
			want: want{
				err: errors.New("instanceCreateError"),
			},
		},
		{
			name: "InstanceCreateSuccess",
			args: args{
				cr: testCMCr(utilCloudManagementParams{statusInstanceID: ""}),
				siExternal: ExternalClientFake{
					createFn: func(mg resource.Managed) (managed.ExternalCreation, error) {
						meta.SetExternalName(mg, defaultInstanceID)
						return managed.ExternalCreation{}, nil
					},
				},
				sInstance: testServiceInstance(defaultInstanceID, defaultInstanceName),
			},
			want: want{
				sID: defaultInstanceID,
			},
		},
		{
			// we should return an error also even if only the binding creation fails
			name: "BindingCreateError",
			args: args{
				cr: defaultCR,
				sbExternal: ExternalClientFake{
					createFn: func(mg resource.Managed) (managed.ExternalCreation, error) {
						return managed.ExternalCreation{}, errors.New("bindingCreateError")
					},
				},
				sInstance: testServiceInstance(defaultInstanceID, defaultInstanceName),
				sBinding:  testServiceBinding(defaultBindingID, defaultBindingName),
			},
			want: want{
				err: errors.New("bindingCreateError"),
				sID: defaultInstanceID,
				bID: "",
			},
		},
		{
			name: "BindingCreateSuccess",
			args: args{
				cr: defaultCR,
				siExternal: ExternalClientFake{
					createFn: func(mg resource.Managed) (managed.ExternalCreation, error) {
						setExternalName(mg, defaultInstanceID)
						return managed.ExternalCreation{}, nil
					},
				},
				sbExternal: ExternalClientFake{
					createFn: func(mg resource.Managed) (managed.ExternalCreation, error) {
						setExternalName(mg, defaultBindingID)
						return managed.ExternalCreation{}, nil
					},
				},
				sInstance: testServiceInstance(defaultInstanceID, defaultInstanceName),
				sBinding:  testServiceBinding(defaultBindingID, defaultBindingName),
			},
			want: want{
				sID: defaultInstanceID,
				bID: defaultBindingID,
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

func TestDeleteResources(t *testing.T) {
	type want struct {
		err error
	}
	type args struct {
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
				sbExternal: ExternalClientFake{
					deleteFn: func() error {
						return errors.New("bindingDeleteError")
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
				sbExternal: ExternalClientFake{
					deleteFn: func() error {
						return nil
					},
				},
				siExternal: ExternalClientFake{
					deleteFn: func() error {
						return errors.New("instanceDeleteError")
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
				sbExternal: ExternalClientFake{
					deleteFn: func() error {
						return nil
					},
				},
				siExternal: ExternalClientFake{
					deleteFn: func() error {
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
			uua := &TfClient{
				siExternal: tc.args.siExternal,
				sbExternal: tc.args.sbExternal,
			}
			err := uua.DeleteResources(context.TODO(), defaultCR)
			if diff := cmp.Diff(err, tc.want.err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.DeleteResources(): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}

// Utils

type utilCloudManagementParams struct {
	extName          string
	siName           string
	sbName           string
	statusInstanceID string
	crName           string
}

func testCMCr(params utilCloudManagementParams) *v1beta1.CloudManagement {
	crName := params.crName
	if crName == "" {
		crName = defaultName
	}

	sm := &v1beta1.CloudManagement{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1beta1.CloudManagementGroupVersionKind.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: crName,
		},
		Spec: v1beta1.CloudManagementSpec{
			ForProvider: v1beta1.CloudManagementParameters{
				SubaccountGuid:      defaultSaId,
				ServiceInstanceName: params.siName,
				ServiceBindingName:  params.sbName,
			},
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: "default",
				},
			},
		},
		Status: v1beta1.CloudManagementStatus{
			AtProvider: v1beta1.CloudManagementObservation{
				DataSourceLookup: &v1beta1.CloudManagementDataSourceLookup{
					CloudManagementPlanID: defaultPlanId,
				},
				ServiceInstanceID: params.statusInstanceID,
			},
		},
	}
	meta.SetExternalName(sm, params.extName)
	return sm
}

func testServiceInstance(siId, siName string) *v1alpha1.SubaccountServiceInstance {

	instance := &v1alpha1.SubaccountServiceInstance{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{},
		},
		Spec: v1alpha1.SubaccountServiceInstanceSpec{},
		Status: v1alpha1.SubaccountServiceInstanceStatus{
			AtProvider: v1alpha1.SubaccountServiceInstanceObservation{
				ID:   &siId,
				Name: &siName,
			},
		},
	}
	meta.SetExternalName(instance, siId)
	return instance
}

func testServiceBinding(sbId, sbName string) *v1alpha1.SubaccountServiceBinding {
	binding := &v1alpha1.SubaccountServiceBinding{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       v1alpha1.SubaccountServiceBindingSpec{},
		Status: v1alpha1.SubaccountServiceBindingStatus{
			AtProvider: v1alpha1.SubaccountServiceBindingObservation{
				ID:   &sbId,
				Name: &sbName,
			},
		},
	}
	meta.SetExternalName(binding, sbId)
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
	deleteFn  func() error
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

func (e ExternalClientFake) Delete(ctx context.Context, mg resource.Managed) error {
	return e.deleteFn()
}

func setExternalName(mg resource.Managed, name string) {
	meta.SetExternalName(mg, name)
}
