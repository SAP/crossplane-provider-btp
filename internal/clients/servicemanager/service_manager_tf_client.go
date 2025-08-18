package servicemanager

import (
	"context"
	"encoding/json"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/pkg/errors"
	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	apisv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"

	"github.com/sap/crossplane-provider-btp/internal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourcesStatus contains a summary of the status of the tf resources managed by the ITfClient
// it uses the crossplane terminology for the external observation and enhances it with IDs of the managed instances
type ResourcesStatus struct {
	managed.ExternalObservation
	InstanceID string
	BindingID  string
}

// ITfClientInitializer will produce the ITfClient used by external
type ITfClientInitializer interface {
	ConnectResources(ctx context.Context, cr *apisv1beta1.ServiceManager) (ITfClient, error)
}

// ITfClient contains domain logic for managing ServiceManager lifecycle
type ITfClient interface {
	ObserveResources(ctx context.Context, cr *apisv1beta1.ServiceManager) (ResourcesStatus, error)
	CreateResources(ctx context.Context, cr *apisv1beta1.ServiceManager) (string, string, error)
	UpdateResources(ctx context.Context, cr *apisv1beta1.ServiceManager) error
	DeleteResources(ctx context.Context, cr *apisv1beta1.ServiceManager) error
}

type Defaults struct {
	InstanceName string `json:"instanceName,omitempty"`
	BindingName  string `json:"bindingName,omitempty"`
}

func NewServiceManagerTfClient(sConnector managed.ExternalConnecter, sbConnector managed.ExternalConnecter, defaults Defaults) *TfClientInitializer {
	return &TfClientInitializer{
		siConnector: sConnector,
		sbConnector: sbConnector,

		defaults: defaults,
	}
}

var _ ITfClientInitializer = &TfClientInitializer{}

type TfClientInitializer struct {
	siConnector managed.ExternalConnecter
	sbConnector managed.ExternalConnecter

	defaults Defaults
}

func (tfI *TfClientInitializer) ConnectResources(ctx context.Context, cr *apisv1beta1.ServiceManager) (ITfClient, error) {
	siInstance := tfI.serviceInstanceCr(cr)
	siExternal, err := tfI.siConnector.Connect(ctx, siInstance)

	if err != nil {
		return nil, err
	}

	siBinding := tfI.serviceBindingCr(cr)
	sbExternal, err := tfI.sbConnector.Connect(ctx, siBinding)

	if err != nil {
		return nil, err
	}
	return &TfClient{
		siExternal: siExternal,
		sInstance:  siInstance,
		sbExternal: sbExternal,
		sBinding:   siBinding,
	}, nil
}

func (tfI *TfClientInitializer) serviceInstanceCr(sm *apisv1beta1.ServiceManager) *apisv1alpha1.SubaccountServiceInstance {
	name := sm.Spec.ForProvider.ServiceInstanceName
	if name == "" {
		name = tfI.defaults.InstanceName
	}

	sInstance := &apisv1alpha1.SubaccountServiceInstance{
		TypeMeta: metav1.TypeMeta{
			Kind:       apisv1alpha1.SubaccountServiceInstance_Kind,
			APIVersion: apisv1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "SERVICE_MANAGER_INSTANCE",
			UID:               sm.UID + "-service-instance",
			DeletionTimestamp: sm.DeletionTimestamp,
		},
		Spec: apisv1alpha1.SubaccountServiceInstanceSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: sm.GetProviderConfigReference().Name,
				},
				ManagementPolicies: []xpv1.ManagementAction{xpv1.ManagementActionAll},
			},
			ForProvider: apisv1alpha1.SubaccountServiceInstanceParameters{
				Name:          &name,
				ServiceplanID: &sm.Status.AtProvider.DataSourceLookup.ServiceManagerPlanID,
				SubaccountID:  internal.Ptr(sm.Spec.ForProvider.SubaccountGuid),
			},
			InitProvider: apisv1alpha1.SubaccountServiceInstanceInitParameters{},
		},
		Status: apisv1alpha1.SubaccountServiceInstanceStatus{},
	}
	sInstanceId, _ := splitExternalName(meta.GetExternalName(sm))
	meta.SetExternalName(sInstance, sInstanceId)
	return sInstance
}

func (tfI *TfClientInitializer) serviceBindingCr(sm *apisv1beta1.ServiceManager) *apisv1alpha1.SubaccountServiceBinding {
	name := sm.Spec.ForProvider.ServiceBindingName
	if name == "" {
		name = tfI.defaults.BindingName
	}

	sBinding := &apisv1alpha1.SubaccountServiceBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       apisv1alpha1.SubaccountServiceBinding_Kind,
			APIVersion: apisv1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "SERVICE_MANAGER_INSTANCE",
			UID:               sm.UID + "-service-binding",
			DeletionTimestamp: sm.DeletionTimestamp,
		},
		Spec: apisv1alpha1.SubaccountServiceBindingSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: sm.GetProviderConfigReference().Name,
				},
				ManagementPolicies: []xpv1.ManagementAction{xpv1.ManagementActionAll},
			},
			ForProvider: apisv1alpha1.SubaccountServiceBindingParameters{
				Name:              &name,
				ServiceInstanceID: internal.Ptr(meta.GetExternalName(sm)),
				SubaccountID:      internal.Ptr(sm.Spec.ForProvider.SubaccountGuid),
			},
		},
		Status: apisv1alpha1.SubaccountServiceBindingStatus{},
	}
	_, sBindingId := splitExternalName(meta.GetExternalName(sm))
	meta.SetExternalName(sBinding, sBindingId)
	return sBinding
}

var _ ITfClient = &TfClient{}

type TfClient struct {
	siExternal managed.ExternalClient
	sbExternal managed.ExternalClient

	sInstance *apisv1alpha1.SubaccountServiceInstance
	sBinding  *apisv1alpha1.SubaccountServiceBinding
}

func (tf *TfClient) DeleteResources(ctx context.Context, cr *apisv1beta1.ServiceManager) error {
	_, err := tf.sbExternal.Delete(ctx, tf.sBinding)
	if err != nil {
		return err
	}
	_, err = tf.siExternal.Delete(ctx, tf.sInstance)
	if err != nil {
		return err
	}
	return nil
}

func (tf *TfClient) UpdateResources(ctx context.Context, cr *apisv1beta1.ServiceManager) error {
	// currently updates are only supported for instances, not bindings
	_, err := tf.siExternal.Update(ctx, tf.sInstance)
	return err
}

// CreateResources creates the service manager instance and binding
// What of the resources need to be created is determined by set IDs in SM's status
func (tf *TfClient) CreateResources(ctx context.Context, cr *apisv1beta1.ServiceManager) (string, string, error) {
	// since instance and binding depend on each other and tf resources are written in Connect() we need to use 2 Create() calls to first create instance and later binding
	// so its expected to do either one of them here
	if cr.Status.AtProvider.ServiceInstanceID == "" {
		sID, err := tf.createInstance(ctx)
		return sID, "", err
	} else {
		bID, err := tf.createBinding(ctx)
		return meta.GetExternalName(tf.sInstance), bID, err
	}
}

func (tf *TfClient) ObserveResources(ctx context.Context, cr *apisv1beta1.ServiceManager) (ResourcesStatus, error) {
	siObs, err := tf.siExternal.Observe(ctx, tf.sInstance)
	if err != nil {
		return ResourcesStatus{}, err
	}
	if !siObs.ResourceExists {
		return ResourcesStatus{
			ExternalObservation: managed.ExternalObservation{ResourceExists: false},
		}, nil
	}
	sbObs, err := tf.sbExternal.Observe(ctx, tf.sBinding)
	if err != nil {
		return ResourcesStatus{}, err
	}
	if !sbObs.ResourceExists {
		return ResourcesStatus{
			ExternalObservation: managed.ExternalObservation{ResourceExists: false},
			InstanceID:          meta.GetExternalName(tf.sInstance),
		}, nil
	}

	conDetails, err := mapTfConnectionDetails(sbObs.ConnectionDetails)
	if err != nil {
		return ResourcesStatus{}, errors.Wrap(err, "Unexpected format of returned connectionDetails")
	}

	// the way the reconciler is implemented we need to do another observe run to actually retrieve if updates are nessecary,
	// the first one is just used to set ready state for any reason, should be rechecked when we have the in-memory clients in place
	// since they reimplement Observe()
	resourceUpToDate := tf.resourcesUpToDate(ctx)

	return ResourcesStatus{
		ExternalObservation: managed.ExternalObservation{
			ResourceExists:    true,
			ResourceUpToDate:  resourceUpToDate,
			ConnectionDetails: conDetails,
		},
		InstanceID: meta.GetExternalName(tf.sInstance),
		BindingID:  meta.GetExternalName(tf.sBinding),
	}, nil
}

// ResourcesUpToDate runs another observe on instance and returns whether they are up to date, currently updates on bindings are not supported
func (tf *TfClient) resourcesUpToDate(ctx context.Context) bool {
	siObs, err := tf.siExternal.Observe(ctx, tf.sInstance)
	return err != nil || siObs.ResourceUpToDate
}

func (tf *TfClient) createInstance(ctx context.Context) (string, error) {
	if _, err := tf.siExternal.Create(ctx, tf.sInstance); err != nil {
		return "", err
	}
	return meta.GetExternalName(tf.sInstance), nil
}

func (tf *TfClient) createBinding(ctx context.Context) (string, error) {
	if _, err := tf.sbExternal.Create(ctx, tf.sBinding); err != nil {
		return "", err
	}
	return meta.GetExternalName(tf.sBinding), nil
}

// splitExternalName splits an externalName into its to part according to the scheme serviceInstanceID/serviceBindingID
// just having the serviceInstanceID is also valid
func splitExternalName(externalName string) (string, string) {
	fragments := strings.Split(externalName, "/")
	if len(fragments) == 2 {
		return fragments[0], fragments[1]
	}
	return fragments[0], ""
}

// mapTfConnectionDetails maps the connection details from the terraform output to the connection details of the CR as expected by the crossplane provider
func mapTfConnectionDetails(conDetails map[string][]byte) (managed.ConnectionDetails, error) {
	bindingAsBytes := conDetails["attribute.credentials"]
	var creds *BindingCredentials
	err := json.Unmarshal(bindingAsBytes, &creds)
	if err != nil {
		return nil, err
	}
	return managed.ConnectionDetails{
		apisv1beta1.ResourceCredentialsClientSecret:      []byte(internal.Val(creds.Clientsecret)),
		apisv1beta1.ResourceCredentialsClientId:          []byte(internal.Val(creds.Clientid)),
		apisv1beta1.ResourceCredentialsServiceManagerUrl: []byte(internal.Val(creds.SmUrl)),
		apisv1beta1.ResourceCredentialsXsuaaUrl:          []byte(internal.Val(creds.Url)),
		apisv1beta1.ResourceCredentialsXsappname:         []byte(internal.Val(creds.Xsappname)),
		apisv1beta1.ResourceCredentialsXsuaaUrlSufix:     []byte("/oauth/token"),
	}, nil
}
