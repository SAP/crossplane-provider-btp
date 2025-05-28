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

// BTPEntitlement implements the Resource interface
type BTPEntitlement struct {
	managedResource *v1alpha1.Entitlement
	externalID      string
}

func (d *BTPEntitlement) GetExternalID() string {
	return d.externalID
}

func (d *BTPEntitlement) GetResourceType() string {
	return "Entitlement"
}

func (d *BTPEntitlement) GetManagedResource() resource.Managed {
	return d.managedResource
}

func (d *BTPEntitlement) SetProviderConfigReference(ref *v1.Reference) {
	d.managedResource.Spec.ProviderConfigReference = ref
}

func (d *BTPEntitlement) SetManagementPolicies(policies []v1.ManagementAction) {
	d.managedResource.Spec.ManagementPolicies = policies
}

// BTPEntitlementAdapter implements the ResourceAdapter interface
type BTPEntitlementAdapter struct{}

func (a *BTPEntitlementAdapter) GetResourceType() string {
	return "Entitlement"
}

func (a *BTPEntitlementAdapter) FetchResources(ctx context.Context, client client.ProviderClient, filter res.ResourceFilter) ([]res.Resource, error) {
	// Get filter criteria
	criteria := filter.GetFilterCriteria()

	// Fetch resources from provider
	providerResources, err := client.GetResourcesByType(ctx, "Entitlement", criteria)
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

func (a *BTPEntitlementAdapter) MapToResource(providerResource interface{}, managementPolicies []v1.ManagementAction) (res.Resource, error) {
	// Cast the provider resource to the BTP entitlement type
	entitlementData, ok := providerResource.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid entitlement data type")
	}

	// Extract fields from the BTP API response
	serviceName, _ := entitlementData["serviceName"].(string)
	// serviceDisplayName is available but not used in the current mapping
	servicePlanName, _ := entitlementData["servicePlanName"].(string)
	servicePlanDisplayName, _ := entitlementData["servicePlanDisplayName"].(string)
	servicePlanUniqueIdentifier, _ := entitlementData["servicePlanUniqueIdentifier"].(string)
	category, _ := entitlementData["category"].(string)
	unlimited, _ := entitlementData["unlimited"].(bool)

	// Handle amount - could be int or int32
	var amount *int
	if amountValue, ok := entitlementData["amount"]; ok && amountValue != nil {
		switch v := amountValue.(type) {
		case int:
			amount = &v
		case int32:
			intVal := int(v)
			amount = &intVal
		case float64:
			intVal := int(v)
			amount = &intVal
		}
	}

	// Create a unique external ID combining service and plan
	externalID := fmt.Sprintf("%s-%s", serviceName, servicePlanName)
	if servicePlanUniqueIdentifier != "" {
		externalID = fmt.Sprintf("%s-%s", serviceName, servicePlanUniqueIdentifier)
	}

	// Use service plan display name for better readability, fallback to plan name
	displayName := servicePlanDisplayName
	if displayName == "" {
		displayName = servicePlanName
	}

	fmt.Printf("- Entitlement: %s (%s) with ID: %s\n", displayName, serviceName, externalID)

	// Map resource
	managedResource := &v1alpha1.Entitlement{}
	managedResource.APIVersion = schema.GroupVersion{Group: "account.btp.sap.crossplane.io", Version: "v1alpha1"}.String()
	managedResource.Kind = "Entitlement"
	managedResource.SetAnnotations(map[string]string{"crossplane.io/external-name": externalID})
	managedResource.SetGenerateName(utils.NormalizeToRFC1123(fmt.Sprintf("%s-%s", serviceName, servicePlanName)))

	managedResource.Labels = map[string]string{
		"btp-service": serviceName,
		"btp-plan":    servicePlanName,
	}

	// Set spec fields from actual BTP API response
	managedResource.Spec.ForProvider.ServiceName = serviceName
	managedResource.Spec.ForProvider.ServicePlanName = servicePlanName

	// Set unique identifier if available
	if servicePlanUniqueIdentifier != "" {
		managedResource.Spec.ForProvider.ServicePlanUniqueIdentifier = &servicePlanUniqueIdentifier
	}

	// Set amount or enable based on the plan type
	if unlimited || category == "ELASTIC_SERVICE" || category == "APPLICATION" {
		// For unlimited or elastic services, use enable instead of amount
		enable := true
		managedResource.Spec.ForProvider.Enable = &enable
	} else if amount != nil {
		// For services with numeric quota, use amount
		managedResource.Spec.ForProvider.Amount = amount
	} else {
		// Default to enable for services without clear quota
		enable := true
		managedResource.Spec.ForProvider.Enable = &enable
	}

	// SubaccountGuid would need to be set based on the target subaccount
	// For now, we'll leave it empty as it needs to be specified during import
	managedResource.Spec.ForProvider.SubaccountGuid = ""

	managedResource.Spec.ManagementPolicies = managementPolicies
	managedResource.Spec.DeletionPolicy = "Orphan"

	return &BTPEntitlement{
		managedResource: managedResource,
		externalID:      externalID,
	}, nil
}

func (a *BTPEntitlementAdapter) PreviewResource(resource res.Resource) {
	entitlement, ok := resource.(*BTPEntitlement)
	if !ok {
		fmt.Println("Invalid resource type provided for preview.")
		return
	}

	const maxWidth = 30

	utils.PrintLine("API Version", entitlement.managedResource.APIVersion, maxWidth)
	utils.PrintLine("Kind", entitlement.managedResource.Kind, maxWidth)
	utils.PrintLine("Name", "<generated on creation>", maxWidth)
	utils.PrintLine("External Name", entitlement.managedResource.Annotations["crossplane.io/external-name"], maxWidth)

	var managementPolicies []string
	for _, policy := range entitlement.managedResource.Spec.ManagementPolicies {
		managementPolicies = append(managementPolicies, string(policy))
	}
	utils.PrintLine("Management Policies", strings.Join(managementPolicies, ", "), maxWidth)

	fmt.Println(strings.Repeat("-", 80))
}
