package btpcli

import "context"

// ListServiceBindings retrieves all service bindings in a subaccount
func (c *BtpCli) ListServiceBindings(ctx context.Context, subaccountID string) ([]ServiceBinding, error) {
	var result []ServiceBinding

	err := c.ExecuteJSON(ctx, &result, "list", "services/binding", "--subaccount", subaccountID)
	if err != nil {
		return nil, err
	}

	return result, nil
}

type ServiceBinding struct {
	Context             *BindingContext     `json:"context,omitempty"`
	CreatedAt           string              `json:"created_at,omitempty"`
	CreatedBy           string              `json:"created_by,omitempty"`
	Credentials         *BindingCredentials `json:"credentials,omitempty"`
	ID                  string              `json:"id,omitempty"`
	Labels              string              `json:"labels,omitempty"`
	Name                string              `json:"name,omitempty"`
	Ready               bool                `json:"ready,omitempty"`
	ServiceInstanceID   string              `json:"service_instance_id,omitempty"`
	ServiceInstanceName string              `json:"service_instance_name,omitempty"`
	SubaccountID        string              `json:"subaccount_id,omitempty"`
	UpdatedAt           string              `json:"updated_at,omitempty"`
}

type BindingCredentials struct {
	ClientID     string `json:"clientid,omitempty"`
	ClientSecret string `json:"clientsecret,omitempty"`
	SmURL        string `json:"sm_url,omitempty"`
	UAADomain    string `json:"uaadomain,omitempty"`
	URL          string `json:"url,omitempty"`
	XSAppName    string `json:"xsappname,omitempty"`
}

type BindingContext struct {
	CrmCustomerID     string `json:"crm_customer_id,omitempty"`
	EnvType           string `json:"env_type,omitempty"`
	GlobalAccountID   string `json:"global_account_id,omitempty"`
	InstanceName      string `json:"instance_name,omitempty"`
	LicenseType       string `json:"license_type,omitempty"`
	Origin            string `json:"origin,omitempty"`
	Platform          string `json:"platform,omitempty"`
	Region            string `json:"region,omitempty"`
	ServiceInstanceID string `json:"service_instance_id,omitempty"`
	SubaccountID      string `json:"subaccount_id,omitempty"`
	Subdomain         string `json:"subdomain,omitempty"`
	ZoneID            string `json:"zone_id,omitempty"`
}
