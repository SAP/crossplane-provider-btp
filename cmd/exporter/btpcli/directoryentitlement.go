package btpcli

import "context"

// ListDirectoryEntitlements retrieves the plans entitled to a directory.
// assignedServices in the same response describes allocations to children,
// rather than the directory's own entitlements.
func (c *BtpCli) ListDirectoryEntitlements(ctx context.Context, directoryID string) ([]DirectoryEntitledService, error) {
	var response struct {
		EntitledServices []DirectoryEntitledService `json:"entitledServices"`
	}
	if err := c.ExecuteJSON(ctx, &response, "list", "accounts/entitlement", "--directory", directoryID); err != nil {
		return nil, err
	}
	return response.EntitledServices, nil
}

type DirectoryEntitledService struct {
	Name         string                         `json:"name,omitempty"`
	ServicePlans []DirectoryEntitledServicePlan `json:"servicePlans,omitempty"`
}

type DirectoryEntitledServicePlan struct {
	Name                 string   `json:"name,omitempty"`
	UniqueIdentifier     string   `json:"uniqueIdentifier,omitempty"`
	Category             string   `json:"category,omitempty"`
	Amount               *float64 `json:"amount,omitempty"`
	AutoAssign           bool     `json:"autoAssign,omitempty"`
	AutoDistributeAmount *float64 `json:"autoDistributeAmount,omitempty"`
}
