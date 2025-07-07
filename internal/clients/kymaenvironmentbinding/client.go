package kymaenvironmentbinding

import (
	"context"

	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
)

type Client interface {
	DescribeInstance(ctx context.Context, kymaInstanceId string) (
		[]provisioningclient.EnvironmentInstanceBindingMetadata,
		error,
	)
	CreateInstance(ctx context.Context, kymaInstanceId string, ttl int) (*Binding, error)
	DeleteInstances(ctx context.Context, bindings []v1alpha1.Binding, kymaInstanceId string) error
}
