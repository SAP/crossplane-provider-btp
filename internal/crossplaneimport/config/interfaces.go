// Package config provides configuration interfaces and types for the crossplane import system.
package config

import (
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/client"
)

// ProviderConfig defines the interface for provider configuration objects.
// This interface is implemented by configuration structures that contain
// provider-specific settings and validation logic.
type ProviderConfig interface {
	// GetProviderConfigRef returns the reference to the provider configuration
	GetProviderConfigRef() client.ProviderConfigRef

	// Validate performs validation on the configuration and returns true if valid
	Validate() bool
}
