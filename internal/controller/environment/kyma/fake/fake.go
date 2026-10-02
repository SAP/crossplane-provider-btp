package fake

import (
	"context"
	"net/http"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	environments "github.com/sap/crossplane-provider-btp/internal/clients/kymaenvironment"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

var _ environments.Client = &MockClient{}

type MockClient struct {
	MockDescribeCluster func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, error)
	MockCreateCluster   func(ctx context.Context, input *v1alpha1.KymaEnvironment) (string, error)
	MockDeleteCluster   func(ctx context.Context, input *v1alpha1.KymaEnvironment) (*http.Response, error)

	// MockSchemaFetcher, if set, overrides the default nilSchemaFetcher.
	// Set it to opt a test into the schema-aware diff path (issue #682);
	// leave it nil to keep the pre-#682 naive-diff behaviour.
	MockSchemaFetcher environments.SchemaFetcher
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
func (c MockClient) SchemaFetcher() environments.SchemaFetcher {
	if c.MockSchemaFetcher != nil {
		return c.MockSchemaFetcher
	}
	return nilSchemaFetcher{}
}

// nilSchemaFetcher returns (nil, nil) so DiffAgainstUpdateSchema takes its
// naive-diff branch. Default when MockSchemaFetcher is unset.
type nilSchemaFetcher struct{}

func (nilSchemaFetcher) GetUpdateSchema(ctx context.Context, environmentType, planName string) (*environments.Schema, error) {
	return nil, nil
}
