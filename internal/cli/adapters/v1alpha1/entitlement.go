package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/cli/pkg/utils"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/importer"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EntitlementAdapter implements the resource.BTPResourceAdapter interface for Entitlement resources.
// It handles fetching entitlements from BTP and converting them to Crossplane managed resources.
type EntitlementAdapter struct{}

// NewEntitlementAdapter creates a new instance of EntitlementAdapter.
func NewEntitlementAdapter() resource.BTPResourceAdapter {
	return &EntitlementAdapter{}
}

// GetName returns the unique identifier for this adapter.
// This name must match the 'type' field used in config.yaml.
func (a *EntitlementAdapter) GetName() string {
	return "Entitlement"
}

// FetchBTPResources fetches entitlements from the BTP environment using the provided client and filters.
func (a *EntitlementAdapter) FetchBTPResources(ctx context.Context, btpClient resource.BTPClientInterface, filters resource.ResourceFilterConfig) ([]resource.BTPResourceRepresentation, error) {
	// Extract filter criteria from the ResourceFilterConfig
	criteria := filters.GetCriteria()

	// Use the concrete ListEntitlements method from BTPClientInterface
	btpResources, err := btpClient.ListEntitlements(ctx, criteria)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch entitlements from BTP: %w", err)
	}

	return btpResources, nil
}

// ConvertToCrossplaneResource converts a single BTP entitlement representation into a Crossplane Entitlement resource.
func (a *EntitlementAdapter) ConvertToCrossplaneResource(ctx context.Context, btpResource resource.BTPResourceRepresentation, providerConfigRefName string, managementPolicy string, transactionID string) (client.Object, error) {
	// Cast the BTP resource to a map for easier handling
	entitlementData, ok := btpResource.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid entitlement data type, expected map[string]interface{}")
	}

	// Extract fields from the BTP API response
	serviceName, _ := entitlementData["serviceName"].(string)
	servicePlanName, _ := entitlementData["servicePlanName"].(string)
	servicePlanDisplayName, _ := entitlementData["servicePlanDisplayName"].(string)
	servicePlanUniqueIdentifier, _ := entitlementData["servicePlanUniqueIdentifier"].(string)
	category, _ := entitlementData["category"].(string)
	unlimited, _ := entitlementData["unlimited"].(bool)

	// Handle amount - could be int, int32, or float64
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

	// Create the Crossplane Entitlement resource
	managedResource := &v1alpha1.Entitlement{}
	managedResource.APIVersion = schema.GroupVersion{Group: "account.btp.sap.crossplane.io", Version: "v1alpha1"}.String()
	managedResource.Kind = "Entitlement"
	managedResource.SetAnnotations(map[string]string{"crossplane.io/external-name": externalID})
	managedResource.SetGenerateName(utils.NormalizeToRFC1123(fmt.Sprintf("%s-%s", serviceName, servicePlanName)))

	managedResource.Labels = map[string]string{
		"btp-service": serviceName,
		"btp-plan":    servicePlanName,
		"import.xpbtp.crossplane.io/transaction-id": transactionID,
	}

	// Set spec fields from BTP API response
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
	} else if amount != nil && *amount > 0 {
		// For services with numeric quota, use amount
		managedResource.Spec.ForProvider.Amount = amount
	} else {
		// Default to enable for services without clear quota
		enable := true
		managedResource.Spec.ForProvider.Enable = &enable
	}

	// SubaccountGuid needs to be extracted from filters or set as required
	// Based on the README, subaccountGuid is a required filter for entitlements
	if subaccountGuid, ok := entitlementData["subaccountGuid"].(string); ok && subaccountGuid != "" {
		managedResource.Spec.ForProvider.SubaccountGuid = subaccountGuid
	} else {
		// This should be provided in the filter criteria as it's required
		return nil, fmt.Errorf("subaccountGuid is required for entitlement resources but was not provided")
	}

	// Set provider config reference
	managedResource.Spec.ProviderConfigReference = &v1.Reference{Name: providerConfigRefName}

	// Set management policy - only use standard Crossplane ManagementActions
	switch managementPolicy {
	case "Observe":
		managedResource.Spec.ManagementPolicies = []v1.ManagementAction{v1.ManagementActionObserve}
	case "*":
		managedResource.Spec.ManagementPolicies = []v1.ManagementAction{v1.ManagementActionAll}
	case "Create":
		managedResource.Spec.ManagementPolicies = []v1.ManagementAction{v1.ManagementActionCreate}
	case "Update":
		managedResource.Spec.ManagementPolicies = []v1.ManagementAction{v1.ManagementActionUpdate}
	case "Delete":
		managedResource.Spec.ManagementPolicies = []v1.ManagementAction{v1.ManagementActionDelete}
	case "LateInitialize":
		managedResource.Spec.ManagementPolicies = []v1.ManagementAction{v1.ManagementActionLateInitialize}
	default:
		// Default to Observe for unknown policies
		managedResource.Spec.ManagementPolicies = []v1.ManagementAction{v1.ManagementActionObserve}
	}

	managedResource.Spec.DeletionPolicy = "Orphan"

	return managedResource, nil
}

// GetFilterSchema returns the schema definition for entitlement filters.
func (a *EntitlementAdapter) GetFilterSchema() resource.FilterSchemaDefinition {
	return resource.FilterSchemaDefinition{
		Fields: map[string]resource.FilterFieldDefinition{
			"subaccountGuid": {
				Type:        "string",
				Description: "The GUID of the subaccount to which the entitlement belongs (required)",
				Required:    true,
			},
			"serviceName": {
				Type:        "string",
				Description: "The name of the entitled service (e.g., 'alert-notification') (required)",
				Required:    true,
			},
			"servicePlanName": {
				Type:        "string",
				Description: "The name of the service plan (e.g., 'standard', 'free') (required)",
				Required:    true,
			},
			"managementPolicies": {
				Type:        "array",
				Description: "How Crossplane should manage the resource (e.g., ['Observe'])",
				Required:    false,
			},
		},
	}
}

// PreviewResource generates a user-friendly preview string for what ConvertToCrossplaneResource would produce.
func (a *EntitlementAdapter) PreviewResource(ctx context.Context, btpResource resource.BTPResourceRepresentation, providerConfigRefName string, managementPolicy string, transactionID string) (string, error) {
	// Cast the BTP resource to a map for easier handling
	entitlementData, ok := btpResource.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid entitlement data type, expected map[string]interface{}")
	}

	// Extract key fields for preview
	serviceName, _ := entitlementData["serviceName"].(string)
	servicePlanName, _ := entitlementData["servicePlanName"].(string)
	servicePlanDisplayName, _ := entitlementData["servicePlanDisplayName"].(string)
	servicePlanUniqueIdentifier, _ := entitlementData["servicePlanUniqueIdentifier"].(string)
	category, _ := entitlementData["category"].(string)
	unlimited, _ := entitlementData["unlimited"].(bool)

	// Handle amount for preview
	var amountStr string
	if amountValue, ok := entitlementData["amount"]; ok && amountValue != nil {
		amountStr = fmt.Sprintf("%v", amountValue)
	}

	// Create external ID for preview
	externalID := fmt.Sprintf("%s-%s", serviceName, servicePlanName)
	if servicePlanUniqueIdentifier != "" {
		externalID = fmt.Sprintf("%s-%s", serviceName, servicePlanUniqueIdentifier)
	}

	// Use service plan display name for better readability
	displayName := servicePlanDisplayName
	if displayName == "" {
		displayName = servicePlanName
	}

	// Determine if enable or amount will be used
	var quotaInfo string
	if unlimited || category == "ELASTIC_SERVICE" || category == "APPLICATION" {
		quotaInfo = "Enable: true (unlimited/elastic service)"
	} else if amountStr != "" && amountStr != "0" {
		quotaInfo = fmt.Sprintf("Amount: %s", amountStr)
	} else {
		quotaInfo = "Enable: true (default)"
	}

	const maxWidth = 30
	var preview strings.Builder

	preview.WriteString("Entitlement Resource Preview:\n")
	preview.WriteString(strings.Repeat("-", 80) + "\n")
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "API Version", "account.btp.sap.crossplane.io/v1alpha1"))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Kind", "Entitlement"))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Name", "<generated on creation>"))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "External Name", externalID))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Service Name", serviceName))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Service Plan Name", servicePlanName))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Service Plan Display Name", displayName))
	if servicePlanUniqueIdentifier != "" {
		preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Plan Unique Identifier", servicePlanUniqueIdentifier))
	}
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Category", category))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Quota Configuration", quotaInfo))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Provider Config Ref", providerConfigRefName))

	// Show the actual management policies that will be set on the resource
	var actualPolicies []string
	switch managementPolicy {
	case "Observe":
		actualPolicies = []string{"Observe"}
	case "*":
		actualPolicies = []string{"*"}
	case "Create":
		actualPolicies = []string{"Create"}
	case "Update":
		actualPolicies = []string{"Update"}
	case "Delete":
		actualPolicies = []string{"Delete"}
	case "LateInitialize":
		actualPolicies = []string{"LateInitialize"}
	default:
		actualPolicies = []string{"Observe"}
	}
	preview.WriteString(fmt.Sprintf("%-*s: %s (from config: %s)\n", maxWidth, "Management Policies", strings.Join(actualPolicies, ", "), managementPolicy))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Transaction ID Label", transactionID))
	preview.WriteString(strings.Repeat("-", 80) + "\n")

	return preview.String(), nil
}

// init registers the EntitlementAdapter with the adapter registry.
func init() {
	importer.RegisterAdapter(NewEntitlementAdapter())
}
