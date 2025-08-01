package fake

import (
	"context"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymamodule"
)

type MockKymaModuleClient struct {
	MockObserve func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error)
	MockCreate  func(moduleName string, moduleChannel string, customResourcePolicy string) error
	MockDelete  func(moduleName string) error
}

// ObserveModule implements KymaModuleClient.ObserveModule
func (m *MockKymaModuleClient) ObserveModule(ctx context.Context, moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
	return m.MockObserve(moduleCr)
}

// CreateModule implements KymaModuleClient.CreateModule
func (m *MockKymaModuleClient) CreateModule(ctx context.Context, moduleName string, moduleChannel string, customResourcePolicy string) error {
	return m.MockCreate(moduleName, moduleChannel, customResourcePolicy)
}

// DeleteModule implements KymaModuleClient.DeleteModule
func (m *MockKymaModuleClient) DeleteModule(ctx context.Context, moduleName string) error {
	return m.MockDelete(moduleName)
}

var _ kymamodule.Client = &MockKymaModuleClient{}
