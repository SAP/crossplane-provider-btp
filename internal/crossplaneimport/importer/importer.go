package importer

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/sap/crossplane-provider-btp/internal/cli/pkg/credentialManager"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/client"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/config"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/kubernetes"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/resource"
	"k8s.io/apimachinery/pkg/runtime"
)

// Importer is the main struct for importing resources
type Importer struct {
	ClientAdapter    client.ClientAdapter
	ResourceAdapters map[string]resource.ResourceAdapter
	ConfigParser     config.ConfigParser
	Scheme           *runtime.Scheme
}

// ImportResources imports resources using the provided adapters
func (i *Importer) ImportResources(ctx context.Context, configPath string, kubeConfigPath string, scheme *runtime.Scheme) ([]resource.Resource, error) {
	// Parse config
	providerConfig, resourceFilters, err := i.ConfigParser.ParseConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate config
	if !providerConfig.Validate() {
		return nil, fmt.Errorf("invalid provider configuration")
	}

	// Get credentials
	creds := credentialManager.RetrieveCredentials()

	// Build client
	client, err := i.ClientAdapter.BuildClient(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("failed to build client: %w", err)
	}

	// Import resources using adapters
	var allResources []resource.Resource
	for _, filter := range resourceFilters {
		resourceType := filter.GetResourceType()
		adapter, exists := i.ResourceAdapters[resourceType]
		if !exists {
			return nil, fmt.Errorf("no adapter found for resource type: %s", resourceType)
		}
		resources, err := adapter.FetchResources(ctx, client, filter)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch resources of type %s: %w", resourceType, err)
		}

		// Set provider config reference and management policies
		for _, res := range resources {
			res.SetProviderConfigReference(&v1.Reference{
				Name: providerConfig.GetProviderConfigRef().Name,
			})
		}

		allResources = append(allResources, resources...)
	}

	return allResources, nil
}

// PreviewResources previews the resources to be imported
func (i *Importer) PreviewResources(resources []resource.Resource) {
	fmt.Println(strings.Repeat("-", 80))
	for _, res := range resources {
		adapter, exists := i.ResourceAdapters[res.GetResourceType()]
		if exists {
			adapter.PreviewResource(res)
		}
	}
}

// CreateResources creates the imported resources in Kubernetes
func (i *Importer) CreateResources(ctx context.Context, resources []resource.Resource, kubeConfigPath string, transactionID string) error {
	// Create Kubernetes client
	k8sClient, err := kubernetes.NewK8sClient(kubeConfigPath, i.Scheme)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Create resources
	for _, res := range resources {
		managedRes := res.GetManagedResource()

		// Add transaction ID annotation
		annotations := managedRes.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations["import-ID"] = transactionID
		managedRes.SetAnnotations(annotations)

		// Create the resource
		err := k8sClient.Create(ctx, managedRes)
		if err != nil {
			return fmt.Errorf("failed to create resource %s: %w", res.GetExternalID(), err)
		}
	}

	return nil
}
