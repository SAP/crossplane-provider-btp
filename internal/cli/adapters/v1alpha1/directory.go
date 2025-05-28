package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/cli/pkg/utils"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/client"
	res "github.com/sap/crossplane-provider-btp/internal/crossplaneimport/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// BTPDirectory implements the Resource interface
type BTPDirectory struct {
	managedResource *v1alpha1.Directory
	externalID      string
}

func (d *BTPDirectory) GetExternalID() string {
	return d.externalID
}

func (d *BTPDirectory) GetResourceType() string {
	return "Directory"
}

func (d *BTPDirectory) GetManagedResource() resource.Managed {
	return d.managedResource
}

func (d *BTPDirectory) SetProviderConfigReference(ref *v1.Reference) {
	d.managedResource.Spec.ProviderConfigReference = ref
}

func (d *BTPDirectory) SetManagementPolicies(policies []v1.ManagementAction) {
	d.managedResource.Spec.ManagementPolicies = policies
}

// BTPDirectoryAdapter implements the ResourceAdapter interface
type BTPDirectoryAdapter struct{}

func (a *BTPDirectoryAdapter) GetResourceType() string {
	return "Directory"
}

func (a *BTPDirectoryAdapter) FetchResources(ctx context.Context, client client.ProviderClient, filter res.ResourceFilter) ([]res.Resource, error) {
	// Get filter criteria
	criteria := filter.GetFilterCriteria()

	// Fetch resources from provider
	providerResources, err := client.GetResourcesByType(ctx, "Directory", criteria)
	if err != nil {
		return nil, err
	}

	// Map to Resource interface
	var resources []res.Resource
	for _, providerResource := range providerResources {
		resource, err := a.MapToResource(providerResource, filter.GetManagementPolicies())
		if err != nil {
			return nil, err
		}
		resources = append(resources, resource)
	}

	return resources, nil
}

func (a *BTPDirectoryAdapter) MapToResource(providerResource interface{}, managementPolicies []v1.ManagementAction) (res.Resource, error) {
	// Cast the provider resource to the BTP directory type
	directoryData, ok := providerResource.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid directory data type")
	}

	// Extract fields from the BTP API response
	guid, _ := directoryData["guid"].(string)
	displayName, _ := directoryData["displayName"].(string)
	description, _ := directoryData["description"].(string)
	subdomain, _ := directoryData["subdomain"].(string)
	// entityState and stateMessage are available but not used in the spec
	// They would be used in status observation if needed

	// Handle directory features
	var directoryFeatures []string
	if features, ok := directoryData["directoryFeatures"].([]string); ok {
		directoryFeatures = features
	} else {
		directoryFeatures = []string{"DEFAULT"} // Default value
	}

	// Handle labels
	var labels map[string][]string
	if labelsData, ok := directoryData["labels"].(map[string][]string); ok {
		labels = labelsData
	}

	fmt.Printf("- Directory: %s with ID: %s\n", displayName, guid)

	// Map resource
	managedResource := &v1alpha1.Directory{}
	managedResource.APIVersion = schema.GroupVersion{Group: "account.btp.sap.crossplane.io", Version: "v1alpha1"}.String()
	managedResource.Kind = "Directory"
	managedResource.SetAnnotations(map[string]string{"crossplane.io/external-name": guid})
	managedResource.SetGenerateName(utils.NormalizeToRFC1123(displayName))

	managedResource.Labels = map[string]string{
		"btp-name": displayName,
	}

	// Set spec fields from actual BTP API response
	managedResource.Spec.ForProvider.DisplayName = &displayName
	managedResource.Spec.ForProvider.Description = description
	managedResource.Spec.ForProvider.DirectoryFeatures = directoryFeatures
	managedResource.Spec.ForProvider.Labels = labels

	// Set subdomain if present
	if subdomain != "" {
		managedResource.Spec.ForProvider.Subdomain = &subdomain
	}

	// DirectoryAdmins would need to be fetched separately or from a different API call
	managedResource.Spec.ForProvider.DirectoryAdmins = []string{} // Will be populated later if available

	managedResource.Spec.ManagementPolicies = managementPolicies
	managedResource.Spec.DeletionPolicy = "Orphan"

	return &BTPDirectory{
		managedResource: managedResource,
		externalID:      guid,
	}, nil
}

func (a *BTPDirectoryAdapter) PreviewResource(resource res.Resource) {
	directory, ok := resource.(*BTPDirectory)
	if !ok {
		fmt.Println("Invalid resource type provided for preview.")
		return
	}

	const maxWidth = 30

	utils.PrintLine("API Version", directory.managedResource.APIVersion, maxWidth)
	utils.PrintLine("Kind", directory.managedResource.Kind, maxWidth)
	utils.PrintLine("Name", "<generated on creation>", maxWidth)
	utils.PrintLine("External Name", directory.managedResource.Annotations["crossplane.io/external-name"], maxWidth)

	var managementPolicies []string
	for _, policy := range directory.managedResource.Spec.ManagementPolicies {
		managementPolicies = append(managementPolicies, string(policy))
	}
	utils.PrintLine("Management Policies", strings.Join(managementPolicies, ", "), maxWidth)

	fmt.Println(strings.Repeat("-", 80))
}
