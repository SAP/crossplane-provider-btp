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

// BTPSubaccount implements the Resource interface
type BTPSubaccount struct {
	managedResource *v1alpha1.Subaccount
	externalID      string
}

func (d *BTPSubaccount) GetExternalID() string {
	return d.externalID
}

func (d *BTPSubaccount) GetResourceType() string {
	return "Subaccount"
}

func (d *BTPSubaccount) GetManagedResource() resource.Managed {
	return d.managedResource
}

func (d *BTPSubaccount) SetProviderConfigReference(ref *v1.Reference) {
	d.managedResource.Spec.ProviderConfigReference = ref
}

func (d *BTPSubaccount) SetManagementPolicies(policies []v1.ManagementAction) {
	d.managedResource.Spec.ManagementPolicies = policies
}

// BTPSubaccountAdapter implements the ResourceAdapter interface
type BTPSubaccountAdapter struct{}

func (a *BTPSubaccountAdapter) GetResourceType() string {
	return "Subaccount"
}

func (a *BTPSubaccountAdapter) FetchResources(ctx context.Context, client client.ProviderClient, filter res.ResourceFilter) ([]res.Resource, error) {
	// Get filter criteria
	criteria := filter.GetFilterCriteria()

	// Fetch resources from provider
	providerResources, err := client.GetResourcesByType(ctx, "Subaccount", criteria)
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

func (a *BTPSubaccountAdapter) MapToResource(providerResource interface{}, managementPolicies []v1.ManagementAction) (res.Resource, error) {
	// Cast the provider resource to the BTP subaccount type
	// This should be the actual BTP API response type
	subaccountData, ok := providerResource.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid subaccount data type")
	}

	// Extract fields from the BTP API response
	guid, _ := subaccountData["guid"].(string)
	displayName, _ := subaccountData["displayName"].(string)
	subdomain, _ := subaccountData["subdomain"].(string)
	region, _ := subaccountData["region"].(string)
	description, _ := subaccountData["description"].(string)

	fmt.Printf("- Subaccount: %s with ID: %s\n", displayName, guid)

	// Map resource
	managedResource := &v1alpha1.Subaccount{}
	managedResource.APIVersion = schema.GroupVersion{Group: "account.btp.sap.crossplane.io", Version: "v1alpha1"}.String()
	managedResource.Kind = "Subaccount"
	managedResource.SetAnnotations(map[string]string{"crossplane.io/external-name": guid})
	managedResource.SetGenerateName(utils.NormalizeToRFC1123(displayName))

	managedResource.Labels = map[string]string{
		"btp-name": displayName,
	}

	// Set spec fields from actual BTP API response
	managedResource.Spec.ForProvider.DisplayName = displayName
	managedResource.Spec.ForProvider.Subdomain = subdomain
	managedResource.Spec.ForProvider.Region = region
	managedResource.Spec.ForProvider.Description = description
	// Note: SubaccountAdmins would need to be fetched separately or from a different API call
	managedResource.Spec.ForProvider.SubaccountAdmins = []string{} // Will be populated later
	managedResource.Spec.ManagementPolicies = managementPolicies
	managedResource.Spec.DeletionPolicy = "Orphan"

	return &BTPSubaccount{
		managedResource: managedResource,
		externalID:      guid,
	}, nil
}

func (a *BTPSubaccountAdapter) PreviewResource(resource res.Resource) {
	subaccount, ok := resource.(*BTPSubaccount)
	if !ok {
		fmt.Println("Invalid resource type provided for preview.")
		return
	}

	const maxWidth = 30

	utils.PrintLine("API Version", subaccount.managedResource.APIVersion, maxWidth)
	utils.PrintLine("Kind", subaccount.managedResource.Kind, maxWidth)
	utils.PrintLine("Name", "<generated on creation>", maxWidth)
	utils.PrintLine("External Name", subaccount.managedResource.Annotations["crossplane.io/external-name"], maxWidth)

	var managementPolicies []string
	for _, policy := range subaccount.managedResource.Spec.ManagementPolicies {
		managementPolicies = append(managementPolicies, string(policy))
	}
	utils.PrintLine("Management Policies", strings.Join(managementPolicies, ", "), maxWidth)

	fmt.Println(strings.Repeat("-", 80))
}
