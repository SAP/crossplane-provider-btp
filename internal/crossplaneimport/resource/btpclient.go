package resource

import (
	"context"
	"fmt"
	"regexp"

	"github.com/sap/crossplane-provider-btp/btp"
)

// BTPClientWrapper wraps the core BTP client and implements BTPClientInterface
// for use in the crossplane import functionality.
type BTPClientWrapper struct {
	client *btp.Client
}

// NewBTPClientWrapper creates a new BTPClientWrapper instance.
func NewBTPClientWrapper(client *btp.Client) BTPClientInterface {
	return &BTPClientWrapper{
		client: client,
	}
}

// GetRawBTPResources fetches resources of a given type based on provided filters.
// This is the generic method that dispatches to specific resource type methods.
func (w *BTPClientWrapper) GetRawBTPResources(ctx context.Context, resourceTypeIdentifier string, filters map[string]string) ([]BTPResourceRepresentation, error) {
	switch resourceTypeIdentifier {
	case "Subaccount":
		return w.ListSubaccounts(ctx, filters)
	case "Entitlement":
		return w.ListEntitlements(ctx, filters)
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceTypeIdentifier)
	}
}

// ListSubaccounts fetches all subaccounts from BTP with optional filtering.
func (w *BTPClientWrapper) ListSubaccounts(ctx context.Context, filters map[string]string) ([]BTPResourceRepresentation, error) {
	// Get all subaccounts from BTP using the accounts service client
	response, _, err := w.client.AccountsServiceClient.SubaccountOperationsAPI.GetSubaccounts(ctx).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to get subaccounts from BTP: %w", err)
	}

	var results []BTPResourceRepresentation
	for _, subaccount := range response.Value {
		// Convert to map for easier handling in the adapter
		subaccountMap := map[string]interface{}{
			"guid":              subaccount.Guid,
			"displayName":       subaccount.DisplayName,
			"subdomain":         subaccount.Subdomain,
			"region":            subaccount.Region,
			"description":       subaccount.Description,
			"state":             subaccount.State,
			"betaEnabled":       subaccount.BetaEnabled,
			"labels":            subaccount.Labels,
			"parentGuid":        subaccount.ParentGUID,
			"globalAccountGuid": subaccount.GlobalAccountGUID,
			"usedForProduction": subaccount.UsedForProduction,
		}

		// Apply filters if provided
		if w.matchesFilter(subaccountMap, filters) {
			results = append(results, subaccountMap)
		}
	}

	return results, nil
}

// ListEntitlements fetches all entitlements from BTP with optional filtering.
// This implementation fetches both entitled and assigned services, and when filtering by subaccountGuid,
// it returns only the services that are actually assigned to that specific subaccount.
func (w *BTPClientWrapper) ListEntitlements(ctx context.Context, filters map[string]string) ([]BTPResourceRepresentation, error) {
	// Extract subaccountGuid from filters for potential filtering
	subaccountGuid, hasSubaccountGuid := filters["subaccountGuid"]

	// Get all entitlements from BTP for the global account
	response, _, err := w.client.EntitlementsServiceClient.GetGlobalAccountAssignments(ctx).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to get entitlements from BTP: %w", err)
	}

	var results []BTPResourceRepresentation

	// Process entitled services (global account entitlements)
	if response.EntitledServices != nil {
		for _, entitlement := range response.EntitledServices {
			if entitlement.ServicePlans != nil {
				for _, servicePlan := range entitlement.ServicePlans {
					// Extract actual values from the API response, handling nil pointers safely
					var serviceName, serviceDisplayName, servicePlanName, servicePlanDisplayName, servicePlanUniqueIdentifier, category string
					var amount, remainingAmount interface{}
					var unlimited interface{}

					// Extract service name
					if entitlement.Name != nil {
						serviceName = *entitlement.Name
					}

					// Extract service display name
					if entitlement.DisplayName != nil {
						serviceDisplayName = *entitlement.DisplayName
					}

					// Extract service plan name
					if servicePlan.Name != nil {
						servicePlanName = *servicePlan.Name
					}

					// Extract service plan display name
					if servicePlan.DisplayName != nil {
						servicePlanDisplayName = *servicePlan.DisplayName
					}

					// Extract service plan unique identifier
					if servicePlan.UniqueIdentifier != nil {
						servicePlanUniqueIdentifier = *servicePlan.UniqueIdentifier
					}

					// Extract category
					if servicePlan.Category != nil {
						category = *servicePlan.Category
					}

					// Extract amount
					if servicePlan.Amount != nil {
						amount = *servicePlan.Amount
					}

					// Extract unlimited flag
					if servicePlan.Unlimited != nil {
						unlimited = *servicePlan.Unlimited
					}

					// Extract remaining amount
					if servicePlan.RemainingAmount != nil {
						remainingAmount = *servicePlan.RemainingAmount
					}

					// Convert to map for easier handling in the adapter
					entitlementMap := map[string]interface{}{
						"serviceName":                 serviceName,
						"serviceDisplayName":          serviceDisplayName,
						"servicePlanName":             servicePlanName,
						"servicePlanDisplayName":      servicePlanDisplayName,
						"servicePlanUniqueIdentifier": servicePlanUniqueIdentifier,
						"category":                    category,
						"amount":                      amount,
						"unlimited":                   unlimited,
						"remainingAmount":             remainingAmount,
						"subaccountGuid":              "", // No specific subaccount for entitled services
					}

					// If no subaccountGuid filter is specified, include entitled services
					// If subaccountGuid filter is specified, skip entitled services (they're not assigned to specific subaccounts)
					if !hasSubaccountGuid || subaccountGuid == "" {
						if w.matchesFilter(entitlementMap, filters) {
							results = append(results, entitlementMap)
						}
					}
				}
			}
		}
	}

	// Process assigned services (services assigned to specific subaccounts/directories)
	if response.AssignedServices != nil {
		for _, assignedService := range response.AssignedServices {
			if assignedService.ServicePlans != nil {
				for _, servicePlan := range assignedService.ServicePlans {
					if servicePlan.AssignmentInfo != nil {
						for _, assignment := range servicePlan.AssignmentInfo {
							// Only include assignments to subaccounts
							if assignment.EntityType != nil && *assignment.EntityType == "SUBACCOUNT" {
								// Extract actual values from the API response, handling nil pointers safely
								var serviceName, serviceDisplayName, servicePlanName, servicePlanDisplayName, servicePlanUniqueIdentifier, category, subaccountGuid string
								var amount interface{}
								var unlimited interface{}

								// Extract service name (required for filtering)
								if assignedService.Name != nil {
									serviceName = *assignedService.Name
								}

								// Extract service display name
								if assignedService.DisplayName != nil {
									serviceDisplayName = *assignedService.DisplayName
								}

								// Extract service plan name (required for filtering)
								if servicePlan.Name != nil {
									servicePlanName = *servicePlan.Name
								}

								// Extract service plan display name
								if servicePlan.DisplayName != nil {
									servicePlanDisplayName = *servicePlan.DisplayName
								}

								// Extract service plan unique identifier
								if servicePlan.UniqueIdentifier != nil {
									servicePlanUniqueIdentifier = *servicePlan.UniqueIdentifier
								}

								// Extract category
								if servicePlan.Category != nil {
									category = *servicePlan.Category
								}

								// Extract subaccount GUID (required for filtering)
								if assignment.EntityId != nil {
									subaccountGuid = *assignment.EntityId
								}

								// Extract amount
								if assignment.Amount != nil {
									amount = *assignment.Amount
								}

								// Extract unlimited flag
								if assignment.UnlimitedAmountAssigned != nil {
									unlimited = *assignment.UnlimitedAmountAssigned
								}

								// Convert to map for easier handling in the adapter
								// This map now contains the actual values from a single, consistent assignment record
								entitlementMap := map[string]interface{}{
									"serviceName":                 serviceName,
									"serviceDisplayName":          serviceDisplayName,
									"servicePlanName":             servicePlanName,
									"servicePlanDisplayName":      servicePlanDisplayName,
									"servicePlanUniqueIdentifier": servicePlanUniqueIdentifier,
									"category":                    category,
									"amount":                      amount,
									"unlimited":                   unlimited,
									"remainingAmount":             nil,            // Not available for assigned services
									"subaccountGuid":              subaccountGuid, // Use actual subaccount GUID from assignment
								}

								// Apply filters if provided (including subaccountGuid, serviceName, servicePlanName)
								// The matchesFilter function will now compare against the actual values from the API response
								if w.matchesFilter(entitlementMap, filters) {
									results = append(results, entitlementMap)
								}
							}
						}
					}
				}
			}
		}
	}

	return results, nil
}

// matchesFilter checks if a resource matches the provided filter criteria.
// Uses regex matching to support patterns.
func (w *BTPClientWrapper) matchesFilter(resource map[string]interface{}, filters map[string]string) bool {
	// If no filters are provided, include all resources
	if len(filters) == 0 {
		return true
	}

	// Apply all filter criteria with AND logic
	for key, expectedValue := range filters {
		if actualValue, exists := resource[key]; exists {
			// Convert actual value to string for comparison
			actualStr := ""
			if actualValue != nil {
				actualStr = fmt.Sprintf("%v", actualValue)
			}

			// Use regex matching to support patterns like ".*"
			matched, err := regexp.MatchString(expectedValue, actualStr)
			if err != nil || !matched {
				return false
			}
		} else {
			// If the field doesn't exist in the resource, it doesn't match
			return false
		}
	}

	return true
}
