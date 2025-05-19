package fake

import (
	"context"

	environments "github.com/sap/crossplane-provider-btp/internal/clients/kymaenvironment"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
)

var _ environments.Client = &MockClient{}

type MockClient struct {
	MockDescribeCluster func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error)
}

func (c MockClient) DescribeInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) (
	*provisioningclient.BusinessEnvironmentInstanceResponseObject,
	error,
) {
	return c.MockDescribeCluster(ctx, &cr)
}
func (c MockClient) CreateInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) error {
	return nil
}
func (c MockClient) UpdateInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) error {
	return nil
}
func (c MockClient) DeleteInstance(ctx context.Context, cr v1alpha1.KymaEnvironment) error {
	return nil
}
