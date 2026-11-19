package btpcli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGlobalAccountHierarchyUnmarshal verifies that the hierarchy response is
// parsed into nested directories and the subaccounts contained at each level,
// including GA-direct subaccounts and nested sub-directories. The response is
// the hierarchy object directly, with no `value` wrapper.
func TestGlobalAccountHierarchyUnmarshal(t *testing.T) {
	const payload = `{
      "guid": "ga",
      "displayName": "Global Account",
      "subaccounts": [
        {"guid": "sa-a", "displayName": "Subaccount-A"}
      ],
      "children": [
        {
          "guid": "dir-1",
          "displayName": "Dir-1",
          "directoryType": "FOLDER",
          "subaccounts": [
            {"guid": "sa-b", "displayName": "Subaccount-B"}
          ],
          "children": [
            {
              "guid": "dir-1-1",
              "displayName": "Dir-1_1",
              "directoryType": "FOLDER",
              "subaccounts": [
                {"guid": "sa-c", "displayName": "Subaccount-C"},
                {"guid": "sa-d", "displayName": "Subaccount-D"}
              ]
            }
          ]
        }
      ]
    }`

	var ga GlobalaccountHierarchy
	require.NoError(t, json.Unmarshal([]byte(payload), &ga))

	require.Equal(t, "ga", ga.GUID)

	// GA-direct subaccount.
	require.Len(t, ga.Subaccounts, 1)
	require.Equal(t, "sa-a", ga.Subaccounts[0].Guid)

	// Top-level directory with its own subaccount and a nested directory.
	require.Len(t, ga.Children, 1)
	dir1 := ga.Children[0]
	require.Equal(t, "dir-1", dir1.GUID)
	require.Len(t, dir1.Subaccounts, 1)
	require.Equal(t, "sa-b", dir1.Subaccounts[0].Guid)

	require.Len(t, dir1.Children, 1)
	dir11 := dir1.Children[0]
	require.Equal(t, "dir-1-1", dir11.GUID)
	require.Len(t, dir11.Subaccounts, 2)
	require.Equal(t, "sa-c", dir11.Subaccounts[0].Guid)
	require.Equal(t, "sa-d", dir11.Subaccounts[1].Guid)
}
