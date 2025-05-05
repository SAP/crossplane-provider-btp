package serviceinstanceclient

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

type TfProxyClientCreator interface {
	Connect(ctx context.Context, cr *v1alpha1.ServiceInstance) (TfProxyClient, error)
}

type TfProxyClient interface {
	Observe(ctx context.Context, cr *v1alpha1.ServiceInstance) (bool, error)
	Create(ctx context.Context, cr *v1alpha1.ServiceInstance) error
	// QueryUpdatedData returns the relevant status data once the async creation is done
	QueryAsyncData(ctx context.Context, cr *v1alpha1.ServiceInstance) *ServiceInstanceData
}

type ServiceInstanceData struct {
	ExternalName string `json:"externalName"`
	ID           string `json:"id"`
}

var _ TfProxyClientCreator = &ServiceInstanceClientCreator{}

type ServiceInstanceClientCreator struct {
	connector managed.ExternalConnecter
}

func NewServiceInstanceClientCreator(connector managed.ExternalConnecter) *ServiceInstanceClientCreator {
	return &ServiceInstanceClientCreator{
		connector: connector,
	}
}

// Connect implements TfProxyClientCreator.
func (s *ServiceInstanceClientCreator) Connect(ctx context.Context, cr *v1alpha1.ServiceInstance) (TfProxyClient, error) {
	ssi := tfServiceInstanceCr(cr)
	ctrl, err := s.connector.Connect(ctx, ssi)
	if err != nil {
		return nil, err
	}

	return &ServiceInstanceClient{
		tfClient: ctrl,
	}, nil
}

var _ TfProxyClient = &ServiceInstanceClient{}

// ServiceInstanceClient is an implementation that provides lifecycle management for service instances
// by interacting with the terraform based resource SubaccountServiceInstance
// it basically behaves as a proxy that maps all the data between our native resource and the tf resource
type ServiceInstanceClient struct {
	tfClient managed.ExternalClient
}

// Create implements TfProxyClient
func (s *ServiceInstanceClient) Create(ctx context.Context, cr *v1alpha1.ServiceInstance) error {
	panic("unimplemented")
}

// Observe implements TfProxyClient
func (s *ServiceInstanceClient) Observe(ctx context.Context, cr *v1alpha1.ServiceInstance) (bool, error) {
	ssi := tfServiceInstanceCr(cr)
	obs, err := s.tfClient.Observe(ctx, ssi)
	if err != nil {
		return false, err
	}
	return obs.ResourceExists, nil
}

// QueryAsyncData implements TfProxyClient
func (s *ServiceInstanceClient) QueryAsyncData(ctx context.Context, cr *v1alpha1.ServiceInstance) *ServiceInstanceData {
	panic("unimplemented")
}

// generates the tf resource for the service instance to run tf operations against
func tfServiceInstanceCr(si *v1alpha1.ServiceInstance) *v1alpha1.SubaccountServiceInstance {
	sInstance := &v1alpha1.SubaccountServiceInstance{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.SubaccountServiceInstance_Kind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: si.Name,
			// make sure no naming conflicts are there for upjet tmp folder creation
			UID:               si.UID + "-service-instance",
			DeletionTimestamp: si.DeletionTimestamp,
		},
		Spec: v1alpha1.SubaccountServiceInstanceSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: pcName(si),
				},
				ManagementPolicies: []xpv1.ManagementAction{xpv1.ManagementActionAll},
			},
			ForProvider: v1alpha1.SubaccountServiceInstanceParameters{
				Name:          &si.Name,
				ServiceplanID: si.Spec.ForProvider.ServiceplanID,
				SubaccountID:  si.Spec.ForProvider.SubaccountID,
			},
			InitProvider: v1alpha1.SubaccountServiceInstanceInitParameters{},
		},
		Status: v1alpha1.SubaccountServiceInstanceStatus{},
	}
	meta.SetExternalName(sInstance, meta.GetExternalName(si))
	return sInstance
}

func pcName(si *v1alpha1.ServiceInstance) string {
	pc := si.GetProviderConfigReference()
	if pc != nil && pc.Name != "" {
		return pc.Name
	}
	return ""
}
