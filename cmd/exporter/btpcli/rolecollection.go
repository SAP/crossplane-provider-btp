package btpcli

import (
	"context"

	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

// The 'btp list security/role-collections' command returns a top-level JSON array,
// not an object wrapping the collections, so we unmarshal directly into the slice.

// ListRoleCollectionsForSubaccounts retrieves role collection info from a subaccount
func (c *BtpCli) ListRoleCollectionsForSubaccounts(ctx context.Context, subaccountID string) ([]RoleCollection, error) {
	var roleCollections []RoleCollection

	err := c.ExecuteJSON(ctx, &roleCollections, "list", "security/role-collections", "--subaccount", subaccountID)
	if err != nil {
		return nil, err
	}

	return roleCollections, nil
}

type RoleCollection struct {
	ParentGUID     string                   `json:"-"`
	Name           string                   `json:"name,omitempty"`
	Description    string                   `json:"description,omitempty"`
	RoleReferences []v1alpha1.RoleReference `json:"roleReferences,omitempty"`
	IsReadOnly     bool                     `json:"isReadOnly,omitempty"`
	CreationType   string                   `json:"creationType,omitempty"`
}
