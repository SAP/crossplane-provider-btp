package fake

import (
	"context"
	"net/http"

	environments "github.com/sap/crossplane-provider-btp/internal/clients/kymaenvironment"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
)

var _ environments.Client = &MockClient{}

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
