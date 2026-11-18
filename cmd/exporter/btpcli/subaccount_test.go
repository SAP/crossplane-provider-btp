package btpcli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRoleCollectionAssignmentsUnmarshal verifies that the role collection user
// assignments response (shared by subaccount and directory admin lookups) is parsed
// into the expected items.
func TestRoleCollectionAssignmentsUnmarshal(t *testing.T) {
	const payload = `{"items":[{"email":"admin1@example.com"},{"email":"admin2@example.com"}]}`

	var resp RoleCollectionAssignmentsResponse
	require.NoError(t, json.Unmarshal([]byte(payload), &resp))

	require.Len(t, resp.Items, 2)
	require.Equal(t, "admin1@example.com", resp.Items[0].Email)
	require.Equal(t, "admin2@example.com", resp.Items[1].Email)
}

// TestDedupRoleCollectionEmails verifies the shared extraction logic: empty emails are
// dropped, duplicates removed, and order preserved.
func TestDedupRoleCollectionEmails(t *testing.T) {
	items := []RoleCollectionUser{
		{Email: "admin1@example.com"},
		{Email: ""}, // technical user without an email: skipped
		{Email: "admin2@example.com"},
		{Email: "admin1@example.com"}, // duplicate: skipped
	}

	got := dedupRoleCollectionEmails(items)
	require.Equal(t, []string{"admin1@example.com", "admin2@example.com"}, got)

	// Never nil, even for an empty input.
	require.NotNil(t, dedupRoleCollectionEmails(nil))
	require.Empty(t, dedupRoleCollectionEmails(nil))
}
