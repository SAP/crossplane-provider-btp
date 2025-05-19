package servicebindingclient

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	ujcontroller "github.com/crossplane/upjet/pkg/controller"
	ujresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CreateTfConnectorFn func(resourceName string, gvk schema.GroupVersionKind, useAsync bool, callbackProvider ujcontroller.CallbackProvider) *ujcontroller.Connector

type SaveConditionsFn func(ctx context.Context, kube client.Client, name string, conditions ...xpv1.Condition) error

type TfProxyClientCreator interface {
	Connect(ctx context.Context, cr *v1alpha1.ServiceBinding) (TfProxyClient, error)
}

type TfProxyClient interface {
	Observe(ctx context.Context) (bool, map[string][]byte, error)
	Create(ctx context.Context) error
	Delete(ctx context.Context) error
	// QueryUpdatedData returns the relevant status data once the async creation is done
	QueryAsyncData(ctx context.Context) *ServiceBindingData
}

type ServiceBindingData struct {
	ExternalName string `json:"externalName"`
	ID           string `json:"id"`
	Conditions   []xpv1.Condition
}

var _ TfProxyClientCreator = &ServiceBindingClientCreator{}

type ServiceBindingClientCreator struct {
	connector managed.ExternalConnecter

	saveConditionsCallback SaveConditionsFn

	connectionPublisher managed.ConnectionPublisher
}

// NewServiceBindingClientCreator creates a connector for the service instance client
// - it uses a callback that creates a tf connector, it defines what resource and configuration it needs via this callback
func NewServiceBindingClientCreator(createConnectorFn CreateTfConnectorFn, saveConditionsCallback SaveConditionsFn, kube client.Client, connectionPublisher managed.ConnectionPublisher) *ServiceBindingClientCreator {
	return &ServiceBindingClientCreator{
		connector: createConnectorFn("btp_subaccount_service_instance",
			v1alpha1.SubaccountServiceBinding_GroupVersionKind,
			true, &APICallbacks{
				saveCallbackFn: saveConditionsCallback,
				kube:           kube,
			}),
		saveConditionsCallback: saveConditionsCallback,
		connectionPublisher:    connectionPublisher,
	}
}

// Connect implements TfProxyClientCreator.
func (s *ServiceBindingClientCreator) Connect(ctx context.Context, cr *v1alpha1.ServiceBinding) (TfProxyClient, error) {
	ssi := tfServiceBindingCr(cr)
	ctrl, err := s.connector.Connect(ctx, ssi)
	if err != nil {
		return nil, err
	}

	return &ServiceBindingClient{
		tfClient:         ctrl,
		tfServiceBinding: ssi,
	}, nil
}

var _ TfProxyClient = &ServiceBindingClient{}

// ServiceBindingClient is an implementation that provides lifecycle management for service instances
// by interacting with the terraform based resource SubaccountServiceBinding
// it basically behaves as a proxy that maps all the data between our native resource and the tf resource
type ServiceBindingClient struct {
	tfClient         managed.ExternalClient
	tfServiceBinding *v1alpha1.SubaccountServiceBinding
}

// Create implements TfProxyClient
func (s *ServiceBindingClient) Create(ctx context.Context) error {
	_, err := s.tfClient.Create(ctx, s.tfServiceBinding)
	return err
}

// Delete implements TfProxyClient
func (s *ServiceBindingClient) Delete(ctx context.Context) error {
	return s.tfClient.Delete(ctx, s.tfServiceBinding)
}

// Observe implements TfProxyClient
func (s *ServiceBindingClient) Observe(ctx context.Context) (bool, map[string][]byte, error) {
	// will return true, true, in case of in memory running async operations
	obs, err := s.tfClient.Observe(ctx, s.tfServiceBinding)
	if err != nil {
		return false, nil, err
	}

	flatDetails, err := flattenSecretData(obs.ConnectionDetails)
	if err != nil {
		return false, nil, err
	}

	return obs.ResourceExists, flatDetails, nil
}

// QueryAsyncData implements TfProxyClient
func (s *ServiceBindingClient) QueryAsyncData(ctx context.Context) *ServiceBindingData {
	// only query the async data if the operation is finished
	if s.tfServiceBinding.GetCondition(ujresource.TypeAsyncOperation).Reason == ujresource.ReasonFinished {
		sid := &ServiceBindingData{}
		sid.ID = internal.Val(s.tfServiceBinding.Status.AtProvider.ID)
		sid.ExternalName = meta.GetExternalName(s.tfServiceBinding)
		sid.Conditions = []xpv1.Condition{xpv1.Available(), ujresource.AsyncOperationFinishedCondition()}
		return sid
	}
	return nil
}

// generates the tf resource for the service instance to run tf operations against
func tfServiceBindingCr(si *v1alpha1.ServiceBinding) *v1alpha1.SubaccountServiceBinding {
	sInstance := &v1alpha1.SubaccountServiceBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.SubaccountServiceBinding_Kind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: si.Name,
			// make sure no naming conflicts are there for upjet tmp folder creation
			UID:               si.UID + "-service-instance",
			DeletionTimestamp: si.DeletionTimestamp,
		},
		Spec: v1alpha1.SubaccountServiceBindingSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: pcName(si),
				},
				ManagementPolicies:               []xpv1.ManagementAction{xpv1.ManagementActionAll},
				WriteConnectionSecretToReference: si.GetWriteConnectionSecretToReference(),
			},
			ForProvider: v1alpha1.SubaccountServiceBindingParameters{
				Name:              &si.Name,
				ServiceInstanceID: si.Spec.ForProvider.ServiceInstanceID,
				SubaccountID:      si.Spec.ForProvider.SubaccountID,
			},
			InitProvider: v1alpha1.SubaccountServiceBindingInitParameters{},
		},
		Status: v1alpha1.SubaccountServiceBindingStatus{},
	}
	meta.SetExternalName(sInstance, meta.GetExternalName(si))
	return sInstance
}

// FlattenSecretData takes a map[string][]byte and flattens any JSON object values into the result map.
// For each key whose value is a JSON object, its keys/values are added to the result map as top-level entries.
// Non-JSON values are kept as-is.
func flattenSecretData(secretData map[string][]byte) (map[string][]byte, error) {
	result := make(map[string][]byte)
	for k, v := range secretData {
		var jsonMap map[string]any
		if err := json.Unmarshal(v, &jsonMap); err == nil {
			for jk, jv := range jsonMap {
				switch val := jv.(type) {
				case string:
					result[jk] = []byte(val)
				default:
					b, err := json.Marshal(val)
					if err != nil {
						return nil, err
					}
					result[jk] = b
				}
			}
		} else {
			result[k] = v
		}
	}
	return result, nil
}

func pcName(si *v1alpha1.ServiceBinding) string {
	pc := si.GetProviderConfigReference()
	if pc != nil && pc.Name != "" {
		return pc.Name
	}
	return ""
}
