package fake

import (
	"context"
	"net/http"

	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	environments "github.com/sap/crossplane-provider-btp/internal/clients/kymaenvironment"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

var _ environments.Client = &MockClient{}
var _ tracking.ReferenceResolverTracker = &MockTracker{}

type MockClient struct {
	MockDescribeCluster func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error)
	MockCreateCluster   func(ctx context.Context, input *v1alpha1.KymaEnvironment) (string, error)
	MockDeleteCluster   func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*http.Response, error)
}

func (c MockClient) DescribeInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) (
	*provisioningclient.BusinessEnvironmentInstanceResponseObject,
	error,
) {
	return c.MockDescribeCluster(ctx, &cr)
}
func (c MockClient) CreateInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) (string, error) {
	return c.MockCreateCluster(ctx, &cr)
}
func (c MockClient) UpdateInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) error {
	return nil
}
func (c MockClient) DeleteInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) (*http.Response, error) {
	if c.MockDeleteCluster != nil {
		return c.MockDeleteCluster(ctx, &cr)
	}
	return nil, nil
}

type MockTracker struct {
	MockDeleteShouldBeBlocked func(mg resource.Managed) bool
}

func (m *MockTracker) Track(ctx context.Context, mg resource.Managed) error {
	return nil
}

func (m *MockTracker) SetConditions(ctx context.Context, mg resource.Managed) {
}

func (m *MockTracker) DeleteShouldBeBlocked(mg resource.Managed) bool {
	if m.MockDeleteShouldBeBlocked != nil {
		return m.MockDeleteShouldBeBlocked(mg)
	}
	return false
}

func (m *MockTracker) ResolveSource(ctx context.Context, ru apisv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error) {
	return nil, nil
}

func (m *MockTracker) ResolveTarget(ctx context.Context, ru apisv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error) {
	return nil, nil
}
