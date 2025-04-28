package fake

import (
	"context"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	environments "github.com/sap/crossplane-provider-btp/internal/clients/cfenvironment"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

type MockClient struct {
	MockDescribeCluster func(cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, error)
	MockCreate          func(cr v1alpha1.CloudFoundryEnvironment) error
	MockDelete          func(cr v1alpha1.CloudFoundryEnvironment) error
	MockUpdate          func(cr v1alpha1.CloudFoundryEnvironment) error

	MockNeedsUpdate func(cr v1alpha1.CloudFoundryEnvironment) bool
}

func (m MockClient) NeedsUpdate(cr v1alpha1.CloudFoundryEnvironment) bool {
	return m.MockNeedsUpdate(cr)
}

func (m MockClient) DescribeInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, error) {
	return m.MockDescribeCluster(cr)
}

func (m MockClient) CreateInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) error {
	return m.MockCreate(cr)
}

func (m MockClient) UpdateInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) error {
	return m.MockUpdate(cr)
}

func (m MockClient) DeleteInstance(ctx context.Context, cr v1alpha1.CloudFoundryEnvironment) error {
	return m.MockDelete(cr)
}

var _ environments.Client = &MockClient{}
