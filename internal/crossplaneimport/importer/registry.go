// Package importer provides the adapter registry functionality for managing
// BTP resource adapters in the crossplane import system.
//
// The registry allows for dynamic registration and retrieval of resource adapters
// that handle specific BTP resource types during the import process.
package importer

import (
	"fmt"
	"sync"

	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/resource"
)

var (
	registeredAdapters = make(map[string]resource.BTPResourceAdapter)
	registryMutex      = &sync.RWMutex{}
)

// RegisterAdapter adds a resource adapter to the registry.
// It is intended to be called from the init() function of adapter implementations.
// It will panic if a nil adapter is provided or if an adapter with the same name
// is already registered.
//
// Example usage:
//
//	func init() {
//	    RegisterAdapter(&SubaccountAdapter{})
//	}
//
// Parameters:
//   - adapter: The BTPResourceAdapter implementation to register
//
// Panics:
//   - If adapter is nil
//   - If adapter.GetName() returns an empty string
//   - If an adapter with the same name is already registered
func RegisterAdapter(adapter resource.BTPResourceAdapter) {
	registryMutex.Lock()
	defer registryMutex.Unlock()

	if adapter == nil {
		panic("cannot register a nil adapter")
	}
	name := adapter.GetName()
	if name == "" {
		panic("cannot register an adapter with an empty name")
	}
	if _, exists := registeredAdapters[name]; exists {
		panic(fmt.Sprintf("adapter with name '%s' already registered", name))
	}
	registeredAdapters[name] = adapter
	// Consider adding logging here if a logger is available globally or passed in.
	// fmt.Printf("Registered adapter: %s\n", name)
}

// GetAdapter retrieves a resource adapter from the registry by its name.
// Returns the adapter and true if found, otherwise nil and false.
//
// This function is thread-safe and can be called concurrently with RegisterAdapter
// and other GetAdapter calls.
//
// Parameters:
//   - resourceTypeName: The name of the resource type to retrieve an adapter for
//
// Returns:
//   - The BTPResourceAdapter for the specified resource type, or nil if not found
//   - A boolean indicating whether the adapter was found
//
// Example usage:
//
//	if adapter, found := GetAdapter("Subaccount"); found {
//	    // Use the adapter to process Subaccount resources
//	    resources, err := adapter.FetchBTPResources(ctx, client, filters)
//	    // ...
//	}
func GetAdapter(resourceTypeName string) (resource.BTPResourceAdapter, bool) {
	registryMutex.RLock()
	defer registryMutex.RUnlock()

	adapter, found := registeredAdapters[resourceTypeName]
	return adapter, found
}

// ListRegisteredAdapters returns a slice of names of all registered adapters.
// This function is useful for debugging, logging, or providing user feedback
// about which resource types are supported.
//
// The returned slice is a copy and can be safely modified by the caller.
// This function is thread-safe and can be called concurrently with other
// registry operations.
//
// Returns:
//   - A slice containing the names of all registered adapters
//
// Example usage:
//
//	adapters := ListRegisteredAdapters()
//	fmt.Printf("Supported resource types: %v\n", adapters)
func ListRegisteredAdapters() []string {
	registryMutex.RLock()
	defer registryMutex.RUnlock()

	names := make([]string, 0, len(registeredAdapters))
	for name := range registeredAdapters {
		names = append(names, name)
	}
	return names
}
