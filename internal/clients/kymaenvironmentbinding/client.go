package kymaenvironmentbinding

import (
	"context"

	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
)

type BindingMetadata = map[string]interface{}

type Client interface {
	DescribeInstance(ctx context.Context, cr v1alpha1.KymaEnvironmentBinding) (
		[]provisioningclient.EnvironmentInstanceBindingMetadata,
		error,
	)
	CreateInstance(ctx context.Context, cr v1alpha1.KymaEnvironmentBinding) (*Binding, error)
	DeleteInstance(ctx context.Context, cr *v1alpha1.KymaEnvironmentBinding) error
}
