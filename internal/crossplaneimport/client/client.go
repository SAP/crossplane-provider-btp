package client

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
)

// ProviderClient represents a client for interacting with a provider
type ProviderClient interface {
	// GetResourcesByType fetches resources of a specific type
	GetResourcesByType(ctx context.Context, resourceType string, filter map[string]string) ([]interface{}, error)
}

// Credentials represents authentication credentials for a provider
type Credentials interface {
	// GetAuthData returns the authentication data
	GetAuthData() map[string][]byte
}

// ClientAdapter adapts provider-specific client creation
type ClientAdapter interface {
	// BuildClient builds a client for the provider
	BuildClient(ctx context.Context, credentials Credentials) (ProviderClient, error)

	// GetCredentials gets credentials for the provider
	GetCredentials(ctx context.Context, kubeConfigPath string, providerConfigRef ProviderConfigRef, scheme *runtime.Scheme) (Credentials, error)
}

// ProviderConfigRef represents a reference to a provider configuration
type ProviderConfigRef struct {
	Name string `yaml:"name"`
}
