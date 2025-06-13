package config

import (
	// Assuming BTPResourceAdapter is in github.com/sap/crossplane-provider-btp/internal/crossplaneimport/resource
	// We need ResourceFilterConfig from there.
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
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
	Tooling               []SubaccountTooling  `yaml:"tooling,omitempty"`
	Imported              []ImportedResource   `yaml:"imported,omitempty"`
}

// SubaccountTooling keeps a reference to the created binding of certain services created to allow API access
type SubaccountTooling struct {
	Subaccount      string               `yaml:"subaccount"`
	SubaccountID    string               `yaml:"subaccountID,omitempty"`
	Kind            string               `yaml:"kind"`
	SecretReference xpv1.SecretReference `yaml:"secretReference,omitempty"`
}

// ImportedResource represents a resource that has been imported into the cluster via cli
type ImportedResource struct {
	Kind      string `yaml:"kind"`
	Name      string `yaml:"name"` // Name of the resource in Kubernetes
	Namespace string `yaml:"namespace,omitempty"`
}

func (c *ImportConfig) AddImported(name, kind string) {

	imported := ImportedResource{
		Kind: kind,
		Name: name,
	}
	c.Imported = append(c.Imported, imported)
}

func (c *ImportConfig) AddTooling(saName, kind, saID string, secretRef xpv1.SecretReference) {
	c.RemoveTooling(saName, kind) // Remove existing tooling if it exists

	tooling := SubaccountTooling{
		Subaccount:      saName,
		Kind:            kind,
		SubaccountID:    saID,
		SecretReference: secretRef,
	}
	c.Tooling = append(c.Tooling, tooling)
}

func (c *ImportConfig) FindTooling(saName, kind string) *SubaccountTooling {
	for _, tooling := range c.Tooling {
		if tooling.Subaccount == saName && tooling.Kind == kind {
			return &tooling
		}
	}
	return nil
}

func (c *ImportConfig) RemoveTooling(saName, kind string) {
	for i, tooling := range c.Tooling {
		if tooling.Subaccount == saName && tooling.Kind == kind {
			c.Tooling = append(c.Tooling[:i], c.Tooling[i+1:]...)
			return
		}
	}
}
