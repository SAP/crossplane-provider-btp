package config

import (
	// Assuming BTPResourceAdapter is in github.com/sap/crossplane-provider-btp/internal/crossplaneimport/resource
	// We need ResourceFilterConfig from there.
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/resource"
)

// ConfiguredResource represents a single resource entry from the config.yaml.
// It defines the type of BTP resource to import, an optional user-defined name
// for this specific import configuration block, and the filters to apply.
type ConfiguredResource struct {
	Type             string                        `yaml:"type"`
	Name             string                        `yaml:"name,omitempty"`             // User-defined name for this import batch/config block
	ManagementPolicy string                        `yaml:"managementPolicy,omitempty"` // Overrides global if set
	Filters          resource.ResourceFilterConfig `yaml:"filters"`                    // This will be map[string]interface{} initially, validated by adapter's schema
}

// ImportConfig represents the overall structure of the config.yaml file used by the CLI.
// It includes global settings like the ProviderConfig reference and default management policy,
// as well as a list of resource configurations to be imported.
type ImportConfig struct {
	ProviderConfigRefName string               `yaml:"providerConfigRefName"` // Name of the ProviderConfig CR in Kubernetes
	ManagementPolicy      string               `yaml:"managementPolicy"`      // Global default management policy
	Resources             []ConfiguredResource `yaml:"resources"`             // List of resources to import
}
