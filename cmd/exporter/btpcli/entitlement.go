package btpcli

import "context"

// ListServiceAssignments retrieves information about services assigned to a subaccount.
func (c *BtpCli) ListServiceAssignments(ctx context.Context, subaccountID string) ([]AssignedService, error) {
	var response EntitlementsBySubaccountResponse

	err := c.ExecuteJSON(ctx, &response, "list", "accounts/entitlement", "--filter-by-subaccount", subaccountID)
	if err != nil {
		return nil, err
	}

	return response.AssignedServices, nil
}

type EntitlementsBySubaccountResponse struct {
	AssignedServices []AssignedService `json:"assignedServices"`
}

type AssignedService struct {
	Name         string                `json:"name,omitempty"`
	DisplayName  string                `json:"displayName,omitempty"`
	IconBase64   string                `json:"iconBase64,omitempty"`
	OwnerType    string                `json:"ownerType,omitempty"`
	ServicePlans []AssignedServicePlan `json:"servicePlans,omitempty"`
}

type AssignedServicePlan struct {
	Name                      string           `json:"name,omitempty"`
	DisplayName               string           `json:"displayName,omitempty"`
	UniqueIdentifier          string           `json:"uniqueIdentifier,omitempty"`
	Category                  string           `json:"category,omitempty"`
	Beta                      bool             `json:"beta,omitempty"`
	MaxAllowedSubaccountQuota *float64         `json:"maxAllowedSubaccountQuota,omitempty"`
	Unlimited                 bool             `json:"unlimited,omitempty"`
	AssignmentInfo            []AssignmentInfo `json:"assignmentInfo,omitempty"`
}

type AssignmentInfo struct {
	EntityID                    string   `json:"entityId,omitempty"`
	EntityType                  string   `json:"entityType,omitempty"`
	Amount                      float64  `json:"amount,omitempty"`
	RequestedAmount             *float64 `json:"requestedAmount,omitempty"`
	EntityState                 string   `json:"entityState,omitempty"`
	StateMessage                string   `json:"stateMessage,omitempty"`
	AutoAssign                  bool     `json:"autoAssign,omitempty"`
	AutoDistributeAmount        *float64 `json:"autoDistributeAmount,omitempty"`
	CreatedDate                 int64    `json:"createdDate,omitempty"`
	ModifiedDate                int64    `json:"modifiedDate,omitempty"`
	Resources                   []string `json:"resources,omitempty"`
	UnlimitedAmountAssigned     bool     `json:"unlimitedAmountAssigned,omitempty"`
	ParentID                    string   `json:"parentId,omitempty"`
	ParentType                  string   `json:"parentType,omitempty"`
	ParentRemainingAmount       *float64 `json:"parentRemainingAmount,omitempty"`
	ParentAmount                float64  `json:"parentAmount,omitempty"`
	AutoAssigned                bool     `json:"autoAssigned,omitempty"`
	BillingObject               *string  `json:"billingObject,omitempty"`
	AvailableBillingObjects     *string  `json:"availableBillingObjects,omitempty"`
	ParentAssignedBillingObject *string  `json:"parentAssignedBillingObject,omitempty"`
}
