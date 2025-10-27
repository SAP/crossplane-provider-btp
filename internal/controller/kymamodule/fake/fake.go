package fake

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	v1alpha1 "github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymamodule"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

type MockSecretFetcher struct {
	MockFetch func(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error)
}

// Fetch implements SecretFetcherInterface.Fetch
func (m *MockSecretFetcher) Fetch(ctx context.Context, cr *v1alpha1.KymaModule) ([]byte, error) {
	return m.MockFetch(ctx, cr)
}

var Client kymamodule.Client = &MockKymaModuleClient{}

// MockTracker allows mock tracker behavior to be mocked with custom functions
// If mock functions are not set, it returns success/no-op by default
type MockTracker struct {
	MockTrack                    func(ctx context.Context, mg resource.Managed) error
	MockDeleteShouldBeBlocked    func(mg resource.Managed) bool
	MockReferenceResolverTracker func(ctx context.Context, mg resource.Managed) error
	MockResolveSource            func(ctx context.Context, usage apisv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error)
	MockResolveTarget            func(ctx context.Context, usage apisv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error)
	MockSetConditions            func(ctx context.Context, mg resource.Managed)
}

func (m *MockTracker) Track(ctx context.Context, mg resource.Managed) error {
	if m.MockTrack != nil {
		return m.MockTrack(ctx, mg)
	}
	// Default: simulate successful tracking
	return nil
}

func (m *MockTracker) DeleteShouldBeBlocked(mg resource.Managed) bool {
	if m.MockDeleteShouldBeBlocked != nil {
		return m.MockDeleteShouldBeBlocked(mg)
	}
	// Default: don't block deletion in tests
	return false
}

func (m *MockTracker) ReferenceResolverTracker(ctx context.Context, mg resource.Managed) error {
	if m.MockReferenceResolverTracker != nil {
		return m.MockReferenceResolverTracker(ctx, mg)
	}
	// Default: simulate successful tracking
	return nil
}

func (m *MockTracker) ResolveSource(ctx context.Context, usage apisv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error) {
	if m.MockResolveSource != nil {
		return m.MockResolveSource(ctx, usage)
	}
	// Default: return nil (not needed in most tests)
	return nil, nil
}

func (m *MockTracker) ResolveTarget(ctx context.Context, usage apisv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error) {
	if m.MockResolveTarget != nil {
		return m.MockResolveTarget(ctx, usage)
	}
	// Default: return nil (not needed in most tests)
	return nil, nil
}

func (m *MockTracker) SetConditions(ctx context.Context, mg resource.Managed) {
	if m.MockSetConditions != nil {
		m.MockSetConditions(ctx, mg)
	}
	// Default: no-op
}
