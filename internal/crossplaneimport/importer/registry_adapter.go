// Package importer provides registry adapter functionality to avoid import cycles.
package importer

import (
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/resource"
)

// RegistryAdapter implements the AdapterRegistry interface for the config package.
// This allows the config package to access the adapter registry without creating
// an import cycle between config and importer packages.
type RegistryAdapter struct{}

// NewRegistryAdapter creates a new RegistryAdapter instance.
//
// Returns:
//   - A new RegistryAdapter instance
func NewRegistryAdapter() *RegistryAdapter {
	return &RegistryAdapter{}
}

// GetAdapter retrieves a resource adapter from the global registry by its name.
// This method delegates to the global GetAdapter function in registry.go.
//
// Parameters:
//   - resourceTypeName: The name of the resource type to retrieve an adapter for
//
// Returns:
//   - The BTPResourceAdapter for the specified resource type, or nil if not found
//   - A boolean indicating whether the adapter was found
func (r *RegistryAdapter) GetAdapter(resourceTypeName string) (resource.BTPResourceAdapter, bool) {
	return GetAdapter(resourceTypeName)
}
