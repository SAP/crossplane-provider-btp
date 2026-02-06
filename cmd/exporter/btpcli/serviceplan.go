package btpcli

import "context"

// ListServicePlans retrieves all service plans in a subaccount
func (c *BtpCli) ListServicePlans(ctx context.Context, subaccountID string) ([]ServicePlan, error) {
	var result []ServicePlan

	err := c.ExecuteJSON(ctx, &result, "list", "services/plan", "--subaccount", subaccountID)
	if err != nil {
		return nil, err
	}

	return result, nil
}

type ServicePlan struct {
	CatalogID           string           `json:"catalog_id,omitempty"`
	CatalogName         string           `json:"catalog_name,omitempty"`
	CreatedAt           string           `json:"created_at,omitempty"`
	DataCenter          string           `json:"data_center,omitempty"`
	Description         string           `json:"description,omitempty"`
	Free                bool             `json:"free,omitempty"`
	ID                  string           `json:"id,omitempty"`
	Labels              string           `json:"labels,omitempty"`
	Metadata            *ServiceMetadata `json:"metadata,omitempty"`
	Name                string           `json:"name,omitempty"`
	Ready               bool             `json:"ready,omitempty"`
	ServiceOfferingID   string           `json:"service_offering_id,omitempty"`
	ServiceOfferingName string           `json:"service_offering_name,omitempty"`
	UpdatedAt           string           `json:"updated_at,omitempty"`
}

type ServiceMetadata struct {
	SupportedPlatforms []string `json:"supportedPlatforms,omitempty"`
}
