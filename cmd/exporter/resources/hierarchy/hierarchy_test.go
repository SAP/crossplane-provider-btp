package hierarchy

import (
	"testing"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/stretchr/testify/require"
)

// testHierarchy models:
//
//	GA (ga)
//	├─ Subaccount-A            (sa-a)
//	└─ Dir-1                   (dir-1)
//	   ├─ Subaccount-B         (sa-b)
//	   └─ Dir-1_1              (dir-1-1)
//	      ├─ Subaccount-C      (sa-c)
//	      └─ Subaccount-D      (sa-d)
func testHierarchy() btpcli.GlobalaccountHierarchy {
	return btpcli.GlobalaccountHierarchy{
		GUID: "ga",
		Subaccounts: []btpcli.HierarchySubaccount{
			{Guid: "sa-a", DisplayName: "Subaccount-A"},
		},
		Children: []btpcli.Child{
			{
				GUID:        "dir-1",
				DisplayName: "Dir-1",
				Subaccounts: []btpcli.HierarchySubaccount{
					{Guid: "sa-b", DisplayName: "Subaccount-B"},
				},
				Children: []btpcli.Child{
					{
						GUID:        "dir-1-1",
						DisplayName: "Dir-1_1",
						Subaccounts: []btpcli.HierarchySubaccount{
							{Guid: "sa-c", DisplayName: "Subaccount-C"},
							{Guid: "sa-d", DisplayName: "Subaccount-D"},
						},
					},
				},
			},
		},
	}
}

func TestAncestorDirectories(t *testing.T) {
	h := New(testHierarchy())

	tests := []struct {
		name    string
		saGUID  string
		wantIDs []string
	}{
		{name: "subaccount under the global account has no ancestors", saGUID: "sa-a", wantIDs: []string{}},
		{name: "subaccount one level deep", saGUID: "sa-b", wantIDs: []string{"dir-1"}},
		{name: "subaccount two levels deep, nearest parent first", saGUID: "sa-c", wantIDs: []string{"dir-1-1", "dir-1"}},
		{name: "unknown subaccount has no ancestors", saGUID: "nope", wantIDs: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.AncestorDirectories(tt.saGUID)
			gotIDs := make([]string, 0, len(got))
			for _, d := range got {
				gotIDs = append(gotIDs, d.GUID)
			}
			require.Equal(t, tt.wantIDs, gotIDs)
		})
	}
}

func TestAncestorDirectoriesForSubaccountsDeduplicates(t *testing.T) {
	h := New(testHierarchy())

	got := h.AncestorDirectoriesForSubaccounts([]string{"sa-b", "sa-c", "sa-a"})

	gotIDs := make([]string, 0, len(got))
	for id := range got {
		gotIDs = append(gotIDs, id)
	}
	require.ElementsMatch(t, []string{"dir-1", "dir-1-1"}, gotIDs)
}

func TestDirectoryK8sResourceName(t *testing.T) {
	h := New(testHierarchy())

	t.Run("resolves a known directory to its sanitized, guid-suffixed name", func(t *testing.T) {
		name, err := h.DirectoryK8sResourceName("dir-1")
		require.NoError(t, err)
		require.Equal(t, "dir-1.dir-1", name)
	})

	t.Run("empty id returns an error", func(t *testing.T) {
		_, err := h.DirectoryK8sResourceName("")
		require.Error(t, err)
	})

	t.Run("unknown id returns an error", func(t *testing.T) {
		_, err := h.DirectoryK8sResourceName("does-not-exist")
		require.Error(t, err)
	})
}

// TestK8sResourceName_CollisionResistance guards the fix for the output
// collision where directories whose display names sanitize to the same value
// (e.g. "Team A" and "team-a") would otherwise map to a single metadata.name.
// Folding the distinct GUIDs into the name keeps them apart.
func TestK8sResourceName_CollisionResistance(t *testing.T) {
	a := K8sResourceName("11111111-1111-1111-1111-111111111111", "Team A")
	b := K8sResourceName("22222222-2222-2222-2222-222222222222", "team-a")

	require.NotEqual(t, a, b, "distinct directories must not collide on the same resource name")
	require.Contains(t, a, "11111111-1111-1111-1111-111111111111")
	require.Contains(t, b, "22222222-2222-2222-2222-222222222222")
}

func TestK8sResourceName_EmptyDisplayName(t *testing.T) {
	require.Equal(t, resources.UndefinedName, K8sResourceName("some-guid", ""))
}
