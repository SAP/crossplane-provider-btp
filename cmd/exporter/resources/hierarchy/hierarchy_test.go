package hierarchy

import (
	"testing"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
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

	t.Run("resolves a known directory to its sanitized name", func(t *testing.T) {
		name, err := h.DirectoryK8sResourceName("dir-1")
		require.NoError(t, err)
		require.Equal(t, "dir-1", name)
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
