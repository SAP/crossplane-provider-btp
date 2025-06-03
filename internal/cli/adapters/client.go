package adapters

import (
	"context"
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/client"
)

// BTPCredentials implements the Credentials interface for BTP
type BTPCredentials struct {
	CISSecretData            []byte
	ServiceAccountSecretData []byte
}

func (c *BTPCredentials) GetAuthData() map[string][]byte {
	return map[string][]byte{
		"cisSecret":            c.CISSecretData,
		"serviceAccountSecret": c.ServiceAccountSecretData,
	}
}

// BTPClient implements the ProviderClient interface for BTP
type BTPClient struct {
	btpClient *btp.Client
}

func (c *BTPClient) GetResourcesByType(ctx context.Context, resourceType string, filter map[string]string) ([]interface{}, error) {
	switch resourceType {
	case "Subaccount":
		return c.getSubaccounts(ctx, filter)
	case "Entitlement":
		return c.getEntitlements(ctx, filter)
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}
}

func (c *BTPClient) getSubaccounts(ctx context.Context, filter map[string]string) ([]interface{}, error) {
	// Get all subaccounts from BTP
	response, _, err := c.btpClient.AccountsServiceClient.SubaccountOperationsAPI.GetSubaccounts(ctx).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to get subaccounts: %w", err)
	}

	var results []interface{}
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

		// Apply filters
		if c.matchesFilter(subaccountMap, filter) {
			results = append(results, subaccountMap)
		}
	}

	return results, nil
}

func (c *BTPClient) getEntitlements(ctx context.Context, filter map[string]string) ([]interface{}, error) {
	// Get all entitlements from BTP for the global account
	response, _, err := c.btpClient.EntitlementsServiceClient.GetGlobalAccountAssignments(ctx).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to get entitlements: %w", err)
	}

	var results []interface{}
	if response.EntitledServices != nil {
		for _, entitlement := range response.EntitledServices {
			if entitlement.ServicePlans != nil {
				for _, servicePlan := range entitlement.ServicePlans {
					// Convert to map for easier handling in the adapter
					entitlementMap := map[string]interface{}{
						"serviceName":                 entitlement.Name,
						"serviceDisplayName":          entitlement.DisplayName,
						"servicePlanName":             servicePlan.Name,
						"servicePlanDisplayName":      servicePlan.DisplayName,
						"servicePlanUniqueIdentifier": servicePlan.UniqueIdentifier,
						"category":                    servicePlan.Category,
						"amount":                      servicePlan.Amount,
						"unlimited":                   servicePlan.Unlimited,
						"remainingAmount":             servicePlan.RemainingAmount,
					}

					// Apply filters
					if c.matchesFilter(entitlementMap, filter) {
						results = append(results, entitlementMap)
					}
				}
			}
		}
	}

	return results, nil
}

func (c *BTPClient) matchesFilter(resource interface{}, filter map[string]string) bool {
	resourceMap, ok := resource.(map[string]interface{})
	if !ok {
		return false
	}

	// Apply all filter criteria with AND logic
	for key, expectedValue := range filter {
		if actualValue, exists := resourceMap[key]; exists {
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

// BTPClientAdapter implements the ClientAdapter interface for BTP
type BTPClientAdapter struct{}

func (a *BTPClientAdapter) BuildClient(ctx context.Context, credentials client.Credentials) (client.ProviderClient, error) {
	btpCreds, ok := credentials.(*BTPCredentials)
	if !ok {
		return nil, fmt.Errorf("invalid credentials type for BTP client")
	}

	// Create BTP client using the same pattern as the controllers
	btpClient, err := btp.NewBTPClient(btpCreds.CISSecretData, btpCreds.ServiceAccountSecretData)
	if err != nil {
		return nil, fmt.Errorf("failed to create BTP client: %w", err)
	}

	return &BTPClient{btpClient: btpClient}, nil
}

func (a *BTPClientAdapter) GetCredentials(ctx context.Context, kubeConfigPath string, providerConfigRef client.ProviderConfigRef, scheme *runtime.Scheme) (client.Credentials, error) {
	// Create a Kubernetes client
	kubeClient, err := createKubeClient(kubeConfigPath, scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Get the ProviderConfig
	pc := &providerv1alpha1.ProviderConfig{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: providerConfigRef.Name}, pc); err != nil {
		return nil, fmt.Errorf("failed to get ProviderConfig: %w", err)
	}

	// Load CIS credentials
	cisSecretData, err := loadCisCredentials(ctx, kubeClient, pc)
	if err != nil {
		return nil, fmt.Errorf("failed to load CIS credentials: %w", err)
	}

	// Load service account credentials
	serviceAccountSecretData, err := loadServiceAccountCredentials(ctx, kubeClient, pc)
	if err != nil {
		return nil, fmt.Errorf("failed to load service account credentials: %w", err)
	}

	return &BTPCredentials{
		CISSecretData:            cisSecretData,
		ServiceAccountSecretData: serviceAccountSecretData,
	}, nil
}

func createKubeClient(kubeConfigPath string, scheme *runtime.Scheme) (kubeclient.Client, error) {
	// Load the kubeconfig from the specified path
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig path %s: %w", kubeConfigPath, err)
	}

	// Create the Kubernetes client with the provided scheme
	kubeClient, err := kubeclient.New(config, kubeclient.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return kubeClient, nil
}

func loadCisCredentials(ctx context.Context, kubeClient kubeclient.Client, pc *providerv1alpha1.ProviderConfig) ([]byte, error) {
	// Use the same logic as in providerconfig package
	cd := pc.Spec.CISSecret
	secretData, err := resource.CommonCredentialExtractor(
		ctx,
		cd.Source,
		kubeClient,
		cd.CommonCredentialSelectors,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to extract CIS credentials: %w", err)
	}
	if secretData == nil {
		return nil, fmt.Errorf("CIS secret is empty")
	}
	return secretData, nil
}

func loadServiceAccountCredentials(ctx context.Context, kubeClient kubeclient.Client, pc *providerv1alpha1.ProviderConfig) ([]byte, error) {
	// Use the same logic as in providerconfig package
	cd := pc.Spec.ServiceAccountSecret
	secretData, err := resource.CommonCredentialExtractor(
		ctx,
		cd.Source,
		kubeClient,
		cd.CommonCredentialSelectors,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to extract service account credentials: %w", err)
	}
	if secretData == nil {
		return nil, fmt.Errorf("service account secret is empty")
	}
	return secretData, nil
}
