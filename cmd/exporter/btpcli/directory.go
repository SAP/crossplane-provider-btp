package btpcli

import "context"

// GetDirectoryAdmins retrieves the usernames assigned to the "Directory Administrator"
// role collection in the given directory. This role collection only exists for
// directories that have the user authorization management (AUTHORIZATIONS) feature
// enabled; callers must not invoke it for plain DEFAULT directories.
func (c *BtpCli) GetDirectoryAdmins(ctx context.Context, directoryID string) ([]string, error) {
	var response RoleCollectionAssignmentsResponse

	err := c.ExecuteJSON(ctx, &response,
		"get", "security/role-collection", "Directory Administrator",
		"--directory", directoryID,
		"--show-user-assignments", "true",
	)
	if err != nil {
		return nil, err
	}

	return dedupRoleCollectionEmails(response.Items), nil
}
