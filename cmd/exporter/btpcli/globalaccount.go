package btpcli

import "context"

// GetGlobalAccountHierarchy retrieves the full global account hierarchy,
// including nested directories and the subaccounts contained in each level.
//
// Note: `get accounts/global-account --show-hierarchy` returns the hierarchy
// object directly at the top level (no `value` wrapper), unlike `list`
// commands. The response is therefore unmarshalled straight into
// GlobalaccountHierarchy.
func (c *BtpCli) GetGlobalAccountHierarchy(ctx context.Context) (GlobalaccountHierarchy, error) {
	var result GlobalaccountHierarchy

	err := c.ExecuteJSON(ctx, &result, "get", "accounts/global-account", "--show-hierarchy")
	if err != nil {
		return GlobalaccountHierarchy{}, err
	}

	return result, nil
}

type Child struct {
	GUID              string                `json:"guid"`
	ParentGuid        string                `json:"parentGuid"`
	ParentGUID        string                `json:"parentGUID"`
	ParentType        string                `json:"parentType"`
	GlobalAccountGUID string                `json:"globalAccountGUID"`
	DisplayName       string                `json:"displayName"`
	Description       string                `json:"description"`
	Subdomain         string                `json:"subdomain"`
	CreatedDate       string                `json:"createdDate"`
	CreatedBy         string                `json:"createdBy"`
	ModifiedDate      string                `json:"modifiedDate"`
	Children          []Child               `json:"children,omitempty"`
	Subaccounts       []HierarchySubaccount `json:"subaccounts,omitempty"`
	EntityState       string                `json:"entityState"`
	StateMessage      string                `json:"stateMessage"`
	DirectoryType     string                `json:"directoryType"`
	DirectoryFeatures []string              `json:"directoryFeatures"`
	CustomProperties  CustomProperties      `json:"customProperties"`
	Labels            map[string][]string   `json:"labels"`
	ContractStatus    string                `json:"contractStatus"`
}

// HierarchySubaccount is a subaccount node as returned inside the global account
// hierarchy (`get accounts/global-account --show-hierarchy`). It is returned both
// at the top level (subaccounts parented directly by the global account) and
// nested under directories. Note: unlike the flat subaccount list, this struct
// does not carry Labels; those are merged in from ListSubaccounts.
type HierarchySubaccount struct {
	Guid              string `json:"guid"`
	TechnicalName     string `json:"technicalName"`
	DisplayName       string `json:"displayName"`
	GlobalAccountGUID string `json:"globalAccountGUID"`
	ParentGUID        string `json:"parentGUID"`
	ParentType        string `json:"parentType"`
	Region            string `json:"region"`
	Subdomain         string `json:"subdomain"`
	BetaEnabled       bool   `json:"betaEnabled"`
	UsedForProduction string `json:"usedForProduction"`
	Description       string `json:"description"`
	State             string `json:"state"`
	StateMessage      string `json:"stateMessage"`
	CreatedDate       string `json:"createdDate"`
	CreatedBy         string `json:"createdBy"`
	ModifiedDate      string `json:"modifiedDate"`
}

// GlobalaccountHiararchy contains the structure of the Globalaccount
type GlobalaccountHierarchy struct {
	CommercialModel  string                `json:"commercialModel"`
	ConsumptionBased bool                  `json:"consumptionBased"`
	LicenseType      string                `json:"licenseType"`
	GeoAccess        string                `json:"geoAccess"`
	CostCenter       string                `json:"costCenter"`
	UseFor           string                `json:"useFor"`
	Origin           string                `json:"origin"`
	GUID             string                `json:"guid"`
	DisplayName      string                `json:"displayName"`
	Description      string                `json:"description"`
	CreatedDate      string                `json:"createdDate"`
	ModifiedDate     string                `json:"modifiedDate"`
	Children         []Child               `json:"children,omitempty"`
	EntityState      string                `json:"entityState"`
	StateMessage     string                `json:"stateMessage"`
	Subdomain        string                `json:"subdomain"`
	Subaccounts      []HierarchySubaccount `json:"subaccounts,omitempty"`
	ContractStatus   string                `json:"contractStatus"`
}

// CustomProperties contains the labels and AccountGUID
type CustomProperties []struct {
	AccountGUID string `json:"accountGUID"`
	Key         string `json:"key"`
	Value       string `json:"value"`
}
