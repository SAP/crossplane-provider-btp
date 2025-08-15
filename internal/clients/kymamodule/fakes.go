package kymamodule

import (
	"context"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
)

type MockKymaModuleClient struct {
	err         error
	apiResponse *v1alpha1.ModuleStatus
}

// ObserveModule implements KymaModuleClient.ObserveModule
func (m *MockKymaModuleClient) ObserveModule(ctx context.Context, moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
	return m.apiResponse, m.err
}

// CreateModule implements KymaModuleClient.CreateModule
func (m *MockKymaModuleClient) CreateModule(ctx context.Context, moduleName string, moduleChannel string, customResourcePolicy string) error {
	return m.err
}

// DeleteModule implements KymaModuleClient.DeleteModule
func (m *MockKymaModuleClient) DeleteModule(ctx context.Context, moduleName string) error {
	return m.err
}

var _ Client = &MockKymaModuleClient{}
