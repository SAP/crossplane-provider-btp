package serviceinstanceclient

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/serviceinstance"
)

var _ TfProxyClient = &ServiceInstanceClient{}

type TfProxyClient = serviceinstance.TfProxyClient

type ServiceInstanceClient struct {
	tfClient managed.ExternalClient
}

// Create implements serviceinstance.TfProxyClient.
func (s *ServiceInstanceClient) Create(ctx context.Context, cr *v1alpha1.ServiceInstance) error {
	panic("unimplemented")
}

// Observe implements serviceinstance.TfProxyClient.
func (s *ServiceInstanceClient) Observe(ctx context.Context, cr *v1alpha1.ServiceInstance) (bool, error) {
	obs, err := s.tfClient.Observe(ctx, cr)
	if err != nil {
		return false, err
	}
	return obs.ResourceExists, nil
}

// QueryAsyncData implements serviceinstance.TfProxyClient.
func (s *ServiceInstanceClient) QueryAsyncData(ctx context.Context, cr *v1alpha1.ServiceInstance) *serviceinstance.ServiceInstanceData {
	panic("unimplemented")
}
