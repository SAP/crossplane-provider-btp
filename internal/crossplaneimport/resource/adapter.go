package resource

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client" // For client.Object
)

// BTPClientInterface defines the methods a BTP client must implement
// to be usable by resource adapters. This interface provides concrete methods
// for fetching different types of BTP resources.
type BTPClientInterface interface {
	// GetRawBTPResources fetches resources of a given type based on provided filters.
	// The filters map allows for flexible querying.
	// Returns a slice of BTPResourceRepresentation, which are generic maps or byte slices.
	GetRawBTPResources(ctx context.Context, resourceTypeIdentifier string, filters map[string]string) ([]BTPResourceRepresentation, error)

	// ListSubaccounts fetches all subaccounts from BTP with optional filtering.
	// Returns a slice of subaccount data as BTPResourceRepresentation.
	ListSubaccounts(ctx context.Context, filters map[string]string) ([]BTPResourceRepresentation, error)

	// ListEntitlements fetches all entitlements from BTP with optional filtering.
	// Returns a slice of entitlement data as BTPResourceRepresentation.
	ListEntitlements(ctx context.Context, filters map[string]string) ([]BTPResourceRepresentation, error)
}

// BTPResourceRepresentation is a generic representation of a fetched BTP resource.
// It's typically a map[string]interface{} or []byte (e.g., raw JSON response)
// that the specific adapter will know how to parse.
type BTPResourceRepresentation interface{} // Can be map[string]interface{}, []byte, or a more structured type if commonalities exist

// ResourceFilterConfig holds the parsed filter configuration for a specific resource type
// as defined in the config.yaml. This will be passed to the adapter.
// The actual structure will vary per resource type.
type ResourceFilterConfig interface {
	// GetCriteria returns the specific filter criteria for the resource type.
	// This might be a map or a more structured type.
	GetCriteria() map[string]string // Example, could be more specific
}

// FilterFieldDefinition describes a single field in a filter schema.
type FilterFieldDefinition struct {
	Type        string // e.g., "string", "integer", "boolean", "array"
	Description string
	Required    bool
}

// FilterSchemaDefinition defines the expected schema for a resource type's
// filter configuration. This can be used for validation and documentation.
// For example, it could be a JSON schema definition.
type FilterSchemaDefinition struct {
	// Schema could be represented as a string (e.g., JSON schema) or a structured Go type.
	Schema string `yaml:"-"` // Example: JSON Schema string, ignore in YAML if this struct is marshalled
	// Fields provides a more structured way to define expected filter fields.
	Fields map[string]FilterFieldDefinition `yaml:"fields,omitempty"`
}

// BTPResourceAdapter defines the contract for components that can handle
// the fetching and conversion of specific BTP resource types.
// This is the new extensible interface as defined in the extensibility implementation plan.
type BTPResourceAdapter interface {
	// GetName returns the unique identifier for the BTP resource type this adapter handles
	// (e.g., "Subaccount", "Entitlement", "ServiceInstance"). This name will be used
	// in config.yaml to identify the resource type.
	GetName() string

	// FetchBTPResources fetches raw resource data from the BTP environment.
	// - btpClient: An interface to interact with BTP APIs.
	// - filters: Specific filter criteria for this resource type, parsed from config.yaml.
	// Returns a slice of BTPResourceRepresentation and an error if any.
	FetchBTPResources(ctx context.Context, btpClient BTPClientInterface, filters ResourceFilterConfig) ([]BTPResourceRepresentation, error)

	// ConvertToCrossplaneResource converts a single raw BTP resource representation
	// into its corresponding Crossplane managed resource object.
	// - btpResource: The raw BTP data for a single resource.
	// - providerConfigRefName: The name of the ProviderConfig to reference.
	// - managementPolicy: The management policy (e.g., "Observe", "*", "Create", "Update", "Delete", "LateInitialize") for the resource.
	// - transactionID: The transaction ID for this import run to be added as a label.
	// Returns a client.Object (the Crossplane managed resource) and an error if any.
	ConvertToCrossplaneResource(ctx context.Context, btpResource BTPResourceRepresentation, providerConfigRefName string, managementPolicy string, transactionID string) (client.Object, error)

	// GetFilterSchema optionally returns a schema definition for the filter
	// configuration expected by this adapter. This can be used by the BTPConfigParser
	// to validate the relevant section in config.yaml and potentially for auto-generating
	// documentation or CLI help for configuring this resource type.
	// If not implemented or not needed, it can return an empty/nil schema or FilterSchemaDefinition with nil Fields.
	GetFilterSchema() FilterSchemaDefinition

	// PreviewResource displays a preview of the Crossplane resource that would be generated
	// from the given BTP resource representation.
	// - transactionID: The transaction ID for this import run (for preview purposes).
	PreviewResource(ctx context.Context, btpResource BTPResourceRepresentation, providerConfigRefName string, managementPolicy string, transactionID string) (string, error)
}
