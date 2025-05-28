package resource

import (
	"context"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/client"
)

// Resource represents a generic resource that can be imported
type Resource interface {
	// GetExternalID returns the ID of the resource in the external system
	GetExternalID() string

	// GetResourceType returns the type of the resource
	GetResourceType() string

	// GetManagedResource returns the Crossplane managed resource
	GetManagedResource() resource.Managed

	// SetProviderConfigReference sets the provider config reference
	SetProviderConfigReference(ref *v1.Reference)

	// SetManagementPolicies sets the management policies for the resource
	SetManagementPolicies(policies []v1.ManagementAction)
}

// ResourceFilter defines criteria for filtering resources
type ResourceFilter interface {
	// GetResourceType returns the type of resource to filter
	GetResourceType() string

	// GetFilterCriteria returns the criteria for filtering resources
	GetFilterCriteria() map[string]string

	// GetManagementPolicies returns the management policies for the resource
	GetManagementPolicies() []v1.ManagementAction
}

// ResourceAdapter adapts provider-specific resources to the Resource interface
type ResourceAdapter interface {
	// GetResourceType returns the type of resource this adapter handles
	GetResourceType() string

	// FetchResources fetches resources from the provider
	FetchResources(ctx context.Context, client client.ProviderClient, filter ResourceFilter) ([]Resource, error)

	// MapToResource maps a provider-specific resource to a Resource
	MapToResource(providerResource interface{}, managementPolicies []v1.ManagementAction) (Resource, error)

	// PreviewResource displays a preview of the resource
	PreviewResource(resource Resource)
}
