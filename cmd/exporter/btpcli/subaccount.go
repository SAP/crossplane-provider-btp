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

// GetSubaccountAdmins retrieves the usernames assigned to the "Subaccount Administrator"
// role collection in the given subaccount.
func (c *BtpCli) GetSubaccountAdmins(ctx context.Context, subaccountID string) ([]string, error) {
	var response RoleCollectionAssignmentsResponse

	err := c.ExecuteJSON(ctx, &response,
		"get", "security/role-collection", "Subaccount Administrator",
		"--subaccount", subaccountID,
		"--show-user-assignments", "true",
	)
	if err != nil {
		return nil, err
	}

	// Only page 1 (up to 500 users) is handled; more than 500 subaccount admins is not expected.
	admins := make([]string, 0, len(response.Items))
	seen := make(map[string]struct{}, len(response.Items))
	for _, u := range response.Items {
		if u.Username == "" {
			continue
		}
		if _, ok := seen[u.Username]; ok {
			continue
		}
		seen[u.Username] = struct{}{}
		admins = append(admins, u.Username)
	}

	return admins, nil
}

type SubaccountsResponse struct {
	Value []Subaccount `json:"value,omitempty"`
}

// RoleCollectionAssignmentsResponse is the result of
// `get security/role-collection <name> --show-user-assignments true`.
type RoleCollectionAssignmentsResponse struct {
	Items []RoleCollectionUser `json:"items,omitempty"`
}

type RoleCollectionUser struct {
	Username string `json:"username,omitempty"`
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
