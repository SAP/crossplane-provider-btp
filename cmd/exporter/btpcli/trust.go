package btpcli

import "context"

// ListSubaccountTrustConfigurations retrieves trust configurations in a subaccount.
func (c *BtpCli) ListSubaccountTrustConfigurations(ctx context.Context, subaccountID string) ([]TrustConfiguration, error) {
	var response []TrustConfiguration
	if err := c.ExecuteJSON(ctx, &response, "list", "security/trust", "--subaccount", subaccountID); err != nil {
		return nil, err
	}
	return response, nil
}

type TrustConfiguration struct {
	Name                         string `json:"name,omitempty"`
	OriginKey                    string `json:"originKey,omitempty"`
	Description                  string `json:"description,omitempty"`
	IdentityProvider             string `json:"identityProvider,omitempty"`
	Domain                       string `json:"domain,omitempty"`
	LinkTextForUserLogon         string `json:"linkTextForUserLogon,omitempty"`
	Status                       string `json:"status,omitempty"`
	AvailableForUserLogon        string `json:"availableForUserLogon,omitempty"`
	CreateShadowUsersDuringLogon string `json:"createShadowUsersDuringLogon,omitempty"`
}
