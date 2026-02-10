package btpcli

import (
	"context"
	"encoding/json"
)

// ListEnvironmentInstances retrieves all environment instances in a subaccount
func (c *BtpCli) ListEnvironmentInstances(ctx context.Context, subaccountID string) ([]EnvironmentInstance, error) {
	var result ListEnvironmentInstancesResponse

	err := c.ExecuteJSON(ctx, &result, "list", "accounts/environment-instance", "--subaccount", subaccountID)
	if err != nil {
		return nil, err
	}

	return result.EnvironmentInstances, nil
}

type ListEnvironmentInstancesResponse struct {
	EnvironmentInstances []EnvironmentInstance `json:"environmentInstances,omitempty"`
}

type EnvironmentInstance struct {
	ID                string                 `json:"id,omitempty"`
	Name              string                 `json:"name,omitempty"`
	BrokerID          string                 `json:"brokerId,omitempty"`
	GlobalAccountGUID string                 `json:"globalAccountGUID,omitempty"`
	SubaccountGUID    string                 `json:"subaccountGUID,omitempty"`
	TenantID          string                 `json:"tenantId,omitempty"`
	ServiceID         string                 `json:"serviceId,omitempty"`
	PlanID            string                 `json:"planId,omitempty"`
	Operation         string                 `json:"operation,omitempty"`
	Parameters        Parameters             `json:"parameters,omitempty"`
	Labels            Labels                 `json:"labels,omitempty"`
	CustomLabels      map[string]interface{} `json:"customLabels,omitempty"`
	Type              string                 `json:"type,omitempty"`
	Status            string                 `json:"status,omitempty"`
	EnvironmentType   string                 `json:"environmentType,omitempty"`
	LandscapeLabel    string                 `json:"landscapeLabel,omitempty"`
	PlatformID        string                 `json:"platformId,omitempty"`
	CreatedDate       string                 `json:"createdDate,omitempty"`
	ModifiedDate      string                 `json:"modifiedDate,omitempty"`
	State             string                 `json:"state,omitempty"`
	StateMessage      string                 `json:"stateMessage,omitempty"`
	ServiceName       string                 `json:"serviceName,omitempty"`
	PlanName          string                 `json:"planName,omitempty"`
}

type Parameters struct {
	InstanceName string `json:"instance_name,omitempty"`
	Status       string `json:"status,omitempty"`
}

func (p *Parameters) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	type params Parameters
	return json.Unmarshal([]byte(s), (*params)(p))
}

type Labels struct {
	APIEndpoint    string `json:"API Endpoint,omitempty"`
	OrgName        string `json:"Org Name,omitempty"`
	OrgID          string `json:"Org ID,omitempty"`
	OrgMemoryLimit string `json:"Org Memory Limit,omitempty"`
}

func (l *Labels) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	type labels Labels
	return json.Unmarshal([]byte(s), (*labels)(l))
}
