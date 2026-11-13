package btpcli

import (
	"context"
	"encoding/json"
)

// ListSubscriptions retrieves applications and their subscription state in a subaccount.
func (c *BtpCli) ListSubscriptions(ctx context.Context, subaccountID string) ([]Subscription, error) {
	var response ListSubscriptionsResponse
	if err := c.ExecuteJSON(ctx, &response, "list", "accounts/subscription", "--subaccount", subaccountID); err != nil {
		return nil, err
	}
	return response.Applications, nil
}

// GetSubscriptionParameters retrieves the user-defined parameters of a subscription.
func (c *BtpCli) GetSubscriptionParameters(ctx context.Context, subaccountID, appName, planName string) (json.RawMessage, error) {
	args := []string{"get", "accounts/subscription", "--subaccount", subaccountID, "--of-app", appName, "--show-parameters", "true"}
	if planName != "" {
		args = append(args, "--plan", planName)
	}

	var parameters json.RawMessage
	if err := c.ExecuteJSON(ctx, &parameters, args...); err != nil {
		return nil, err
	}
	return parameters, nil
}

type ListSubscriptionsResponse struct {
	Applications []Subscription `json:"applications,omitempty"`
}

type Subscription struct {
	AppName               string `json:"appName,omitempty"`
	PlanName              string `json:"planName,omitempty"`
	State                 string `json:"state,omitempty"`
	SubscriptionGUID      string `json:"subscriptionGUID,omitempty"`
	SupportsGetParameters bool   `json:"supportsGetParameters,omitempty"`
}
