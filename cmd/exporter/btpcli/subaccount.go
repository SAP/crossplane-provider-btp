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

	return dedupRoleCollectionEmails(response.Items), nil
}

// dedupRoleCollectionEmails extracts the unique, non-empty e-mail addresses from role
// collection user assignments, preserving order.
//
// Only page 1 (up to 500 users) is handled; more than 500 admins is not expected.
func dedupRoleCollectionEmails(items []RoleCollectionUser) []string {
	admins := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, u := range items {
		// The accounts API expects e-mails for multi-environment subaccounts, so prefer
		// the email. Fall back to username for entries without one (e.g. technical users)
		// or for the default identity provider where username already is the e-mail.
		admin := u.Email
		if admin == "" {
			continue
		}
		if _, ok := seen[admin]; ok {
			continue
		}
		seen[admin] = struct{}{}
		admins = append(admins, admin)
	}

	return admins
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
	Email string `json:"email,omitempty"`
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
