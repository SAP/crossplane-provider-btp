package btpcli

import "context"

// ListServiceInstances retrieves all service instances in a subaccount
func (c *BtpCli) ListServiceInstances(ctx context.Context, subaccountID string) ([]ServiceInstance, error) {
	var result []ServiceInstance

	err := c.ExecuteJSON(ctx, &result, "list", "services/instance", "--subaccount", subaccountID)
	if err != nil {
		return nil, err
	}

	return result, nil
}

type ServiceInstance struct {
	ID            string        `json:"id,omitempty"`
	Ready         bool          `json:"ready,omitempty"`
	LastOperation LastOperation `json:"last_operation,omitempty"`
	Name          string        `json:"name,omitempty"`
	ServicePlanID string        `json:"service_plan_id,omitempty"`
	PlatformID    string        `json:"platform_id,omitempty"`
	Context       Context       `json:"context,omitempty"`
	Usable        bool          `json:"usable,omitempty"`
	SubaccountID  string        `json:"subaccount_id,omitempty"`
	Protected     *bool         `json:"protected,omitempty"`
	CreatedBy     string        `json:"created_by,omitempty"`
	CreatedAt     string        `json:"created_at,omitempty"`
	UpdatedAt     string        `json:"updated_at,omitempty"`
	Labels        string        `json:"labels,omitempty"`
}

type LastOperation struct {
	ID                  string `json:"id,omitempty"`
	Ready               bool   `json:"ready,omitempty"`
	Type                string `json:"type,omitempty"`
	State               string `json:"state,omitempty"`
	ResourceID          string `json:"resource_id,omitempty"`
	ResourceType        string `json:"resource_type,omitempty"`
	PlatformID          string `json:"platform_id,omitempty"`
	CorrelationID       string `json:"correlation_id,omitempty"`
	Reschedule          bool   `json:"reschedule,omitempty"`
	RescheduleTimestamp string `json:"reschedule_timestamp,omitempty"`
	DeletionScheduled   string `json:"deletion_scheduled,omitempty"`
	CreatedAt           string `json:"created_at,omitempty"`
	UpdatedAt           string `json:"updated_at,omitempty"`
}

type Context struct {
	Origin          string `json:"origin,omitempty"`
	Region          string `json:"region,omitempty"`
	ZoneID          string `json:"zone_id,omitempty"`
	EnvType         string `json:"env_type,omitempty"`
	Platform        string `json:"platform,omitempty"`
	Subdomain       string `json:"subdomain,omitempty"`
	LicenseType     string `json:"license_type,omitempty"`
	InstanceName    string `json:"instance_name,omitempty"`
	SubaccountID    string `json:"subaccount_id,omitempty"`
	CRMCustomerID   string `json:"crm_customer_id,omitempty"`
	GlobalAccountID string `json:"global_account_id,omitempty"`
}
