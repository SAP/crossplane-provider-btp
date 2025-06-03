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

// SubaccountAdapter implements the resource.BTPResourceAdapter interface for Subaccount resources.
// It handles fetching subaccounts from BTP and converting them to Crossplane managed resources.
type SubaccountAdapter struct{}

// NewSubaccountAdapter creates a new instance of SubaccountAdapter.
func NewSubaccountAdapter() resource.BTPResourceAdapter {
	return &SubaccountAdapter{}
}

// GetName returns the unique identifier for this adapter.
// This name must match the 'type' field used in config.yaml.
func (a *SubaccountAdapter) GetName() string {
	return "Subaccount"
}

// FetchBTPResources fetches subaccounts from the BTP environment using the provided client and filters.
func (a *SubaccountAdapter) FetchBTPResources(ctx context.Context, btpClient resource.BTPClientInterface, filters resource.ResourceFilterConfig) ([]resource.BTPResourceRepresentation, error) {
	// Extract filter criteria from the ResourceFilterConfig
	criteria := filters.GetCriteria()

	// Use the concrete ListSubaccounts method from BTPClientInterface
	btpResources, err := btpClient.ListSubaccounts(ctx, criteria)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch subaccounts from BTP: %w", err)
	}

	return btpResources, nil
}

// ConvertToCrossplaneResource converts a single BTP subaccount representation into a Crossplane Subaccount resource.
func (a *SubaccountAdapter) ConvertToCrossplaneResource(ctx context.Context, btpResource resource.BTPResourceRepresentation, providerConfigRefName string, managementPolicy string, transactionID string) (client.Object, error) {
	// Cast the BTP resource to a map for easier handling
	subaccountData, ok := btpResource.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid subaccount data type, expected map[string]interface{}")
	}

	// Extract fields from the BTP API response
	guid, _ := subaccountData["guid"].(string)
	displayName, _ := subaccountData["displayName"].(string)
	subdomain, _ := subaccountData["subdomain"].(string)
	region, _ := subaccountData["region"].(string)
	description, _ := subaccountData["description"].(string)

	// Handle optional fields
	var betaEnabled bool
	if beta, ok := subaccountData["betaEnabled"].(bool); ok {
		betaEnabled = beta
	}

	var usedForProduction string
	if prod, ok := subaccountData["usedForProduction"].(string); ok {
		usedForProduction = prod
	}

	// Handle labels
	var labels map[string][]string
	if labelData, ok := subaccountData["labels"].(map[string]interface{}); ok {
		labels = make(map[string][]string)
		for key, value := range labelData {
			if valueSlice, ok := value.([]interface{}); ok {
				var stringSlice []string
				for _, v := range valueSlice {
					if s, ok := v.(string); ok {
						stringSlice = append(stringSlice, s)
					}
				}
				labels[key] = stringSlice
			}
		}
	}

	fmt.Printf("- Subaccount: %s with ID: %s\n", displayName, guid)

	// Create the Crossplane Subaccount resource
	managedResource := &v1alpha1.Subaccount{}
	managedResource.APIVersion = schema.GroupVersion{Group: "account.btp.sap.crossplane.io", Version: "v1alpha1"}.String()
	managedResource.Kind = "Subaccount"
	managedResource.SetAnnotations(map[string]string{"crossplane.io/external-name": guid})
	managedResource.SetGenerateName(utils.NormalizeToRFC1123(displayName))

	managedResource.Labels = map[string]string{
		"btp-name": displayName,
		"import.xpbtp.crossplane.io/transaction-id": transactionID,
	}

	// Set spec fields from BTP API response
	managedResource.Spec.ForProvider.DisplayName = displayName
	managedResource.Spec.ForProvider.Subdomain = subdomain
	managedResource.Spec.ForProvider.Region = region
	managedResource.Spec.ForProvider.Description = description
	managedResource.Spec.ForProvider.BetaEnabled = betaEnabled
	managedResource.Spec.ForProvider.UsedForProduction = usedForProduction
	if labels != nil {
		managedResource.Spec.ForProvider.Labels = labels
	}

	// Handle subaccountAdmins - this is a required field, so we need to provide a placeholder if not available
	var subaccountAdmins []string
	if admins, ok := subaccountData["_subaccountAdmins"]; ok {
		if adminsSlice, ok := admins.([]string); ok {
			subaccountAdmins = adminsSlice
		} else if adminsIface, ok := admins.([]interface{}); ok {
			// Convert []interface{} to []string
			for _, v := range adminsIface {
				if s, ok := v.(string); ok {
					subaccountAdmins = append(subaccountAdmins, s)
				}
			}
		}
	}
	if len(subaccountAdmins) > 0 {
		managedResource.Spec.ForProvider.SubaccountAdmins = subaccountAdmins
	} else {
		placeholder := "placeholder-admin@example.com"
		fmt.Printf("WARNING: No subaccountAdmins specified for subaccount %s (%s). Using placeholder: %s\n", displayName, guid, placeholder)
		managedResource.Spec.ForProvider.SubaccountAdmins = []string{placeholder}
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

// GetFilterSchema returns the schema definition for subaccount filters.
func (a *SubaccountAdapter) GetFilterSchema() resource.FilterSchemaDefinition {
	return resource.FilterSchemaDefinition{
		Fields: map[string]resource.FilterFieldDefinition{
			"displayName": {
				Type:        "string",
				Description: "Filter subaccounts by display name (supports regex patterns)",
				Required:    false,
			},
			"region": {
				Type:        "string",
				Description: "Filter subaccounts by region (supports regex patterns)",
				Required:    false,
			},
			"subdomain": {
				Type:        "string",
				Description: "Filter subaccounts by subdomain (supports regex patterns)",
				Required:    false,
			},
			"labels": {
				Type:        "object",
				Description: "Filter subaccounts by labels (key-value pairs)",
				Required:    false,
			},
			"usedForProduction": {
				Type:        "string",
				Description: "Filter subaccounts by production usage (NOT_USED_FOR_PRODUCTION, USED_FOR_PRODUCTION, UNSET)",
				Required:    false,
			},
			"betaEnabled": {
				Type:        "boolean",
				Description: "Filter subaccounts by beta services enablement",
				Required:    false,
			},
		},
	}
}

// PreviewResource generates a user-friendly preview string for what ConvertToCrossplaneResource would produce.
func (a *SubaccountAdapter) PreviewResource(ctx context.Context, btpResource resource.BTPResourceRepresentation, providerConfigRefName string, managementPolicy string, transactionID string) (string, error) {
	// Cast the BTP resource to a map for easier handling
	subaccountData, ok := btpResource.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid subaccount data type, expected map[string]interface{}")
	}

	// Extract key fields for preview
	guid, _ := subaccountData["guid"].(string)
	displayName, _ := subaccountData["displayName"].(string)
	subdomain, _ := subaccountData["subdomain"].(string)
	region, _ := subaccountData["region"].(string)
	description, _ := subaccountData["description"].(string)

	const maxWidth = 30
	var preview strings.Builder

	preview.WriteString("Subaccount Resource Preview:\n")
	preview.WriteString(strings.Repeat("-", 80) + "\n")
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "API Version", "account.btp.sap.crossplane.io/v1alpha1"))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Kind", "Subaccount"))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Name", "<generated on creation>"))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "External Name", guid))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Display Name", displayName))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Subdomain", subdomain))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Region", region))
	preview.WriteString(fmt.Sprintf("%-*s: %s\n", maxWidth, "Description", description))
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

// init registers the SubaccountAdapter with the adapter registry.
func init() {
	importer.RegisterAdapter(NewSubaccountAdapter())
}
