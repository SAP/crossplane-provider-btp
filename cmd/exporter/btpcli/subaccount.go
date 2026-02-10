package btpcli

import "context"

// ListSubaccounts retrieves all subaccounts the logged-in user has permission for in the current global subaccount.
func (c *BtpCli) ListSubaccounts(ctx context.Context) ([]Subaccount, error) {
	var response SubaccountsResponse

	err := c.ExecuteJSON(ctx, &response, "list", "accounts/subaccount", "--authorized")
	if err != nil {
		return nil, err
	}

	return response.Value, nil
}

type SubaccountsResponse struct {
	Value []Subaccount `json:"value,omitempty"`
}

type Subaccount struct {
	GUID              string              `json:"guid,omitempty"`
	TechnicalName     string              `json:"technicalName,omitempty"`
	DisplayName       string              `json:"displayName,omitempty"`
	GlobalAccountGUID string              `json:"globalAccountGUID,omitempty"`
	ParentGUID        string              `json:"parentGUID,omitempty"`
	ParentType        string              `json:"parentType,omitempty"`
	Region            string              `json:"region,omitempty"`
	Subdomain         string              `json:"subdomain,omitempty"`
	BetaEnabled       bool                `json:"betaEnabled,omitempty"`
	UsedForProduction string              `json:"usedForProduction,omitempty"`
	Description       string              `json:"description,omitempty"`
	State             string              `json:"state,omitempty"`
	StateMessage      string              `json:"stateMessage,omitempty"`
	Labels            map[string][]string `json:"labels,omitempty"`
	CreatedDate       string              `json:"createdDate,omitempty"`
	CreatedBy         string              `json:"createdBy,omitempty"`
	ModifiedDate      string              `json:"modifiedDate,omitempty"`
}
