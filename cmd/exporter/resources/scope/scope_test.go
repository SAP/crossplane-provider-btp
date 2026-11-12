package scope

import (
	"sort"
	"testing"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/stretchr/testify/require"
)

// referenceHierarchy models:
//
//	Global Account
//	└ Subaccount-A            (parent = GA)
//	└ Dir-1                   (parent = GA)
//	  └ Subaccount-B          (parent = Dir-1)
//	  └ Dir-1_1               (parent = Dir-1)
//	    └ Subaccount-C        (parent = Dir-1_1)
//	    └ Subaccount-D        (parent = Dir-1_1)
func referenceHierarchy() btpcli.GlobalaccountHierarchy {
	return btpcli.GlobalaccountHierarchy{
		GUID: "ga",
		Subaccounts: []btpcli.HierarchySubaccount{
			{Guid: "sa-a", DisplayName: "Subaccount-A"},
		},
		Children: []btpcli.Child{
			{
				GUID:          "dir-1",
				DisplayName:   "Dir-1",
				DirectoryType: "DIRECTORY",
				Subaccounts: []btpcli.HierarchySubaccount{
					{Guid: "sa-b", DisplayName: "Subaccount-B"},
				},
				Children: []btpcli.Child{
					{
						GUID:          "dir-1-1",
						DisplayName:   "Dir-1_1",
						DirectoryType: "DIRECTORY",
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

func sorted(s []string) []string {
	sort.Strings(s)
	return s
}

func TestNewHierarchy_FlattensAllNodes(t *testing.T) {
	h := NewHierarchy(referenceHierarchy())

	dirIDs := make([]string, 0)
	for _, d := range h.AllDirectories() {
		dirIDs = append(dirIDs, d.GUID)
	}
	saIDs := make([]string, 0)
	for _, sa := range h.AllSubaccounts() {
		saIDs = append(saIDs, sa.Guid)
	}

	require.Equal(t, []string{"dir-1", "dir-1-1"}, sorted(dirIDs))
	require.Equal(t, []string{"sa-a", "sa-b", "sa-c", "sa-d"}, sorted(saIDs))
}

func TestExpand(t *testing.T) {
	tests := []struct {
		name      string
		selected  []string
		wantSubs  []string
		wantDirs  []string
		wantRoots int
	}{
		{
			name:      "parent dir expands to all descendants",
			selected:  []string{"dir-1"},
			wantSubs:  []string{"sa-b", "sa-c", "sa-d"},
			wantDirs:  []string{"dir-1", "dir-1-1"},
			wantRoots: 1,
		},
		{
			name:      "nested dir expands to its subtree only",
			selected:  []string{"dir-1-1"},
			wantSubs:  []string{"sa-c", "sa-d"},
			wantDirs:  []string{"dir-1-1"},
			wantRoots: 1,
		},
		{
			name:      "dir plus explicit leaf does not duplicate",
			selected:  []string{"dir-1", "sa-b"},
			wantSubs:  []string{"sa-b", "sa-c", "sa-d"},
			wantDirs:  []string{"dir-1", "dir-1-1"},
			wantRoots: 2,
		},
		{
			name:      "GA-direct subaccount selectable standalone",
			selected:  []string{"sa-a"},
			wantSubs:  []string{"sa-a"},
			wantDirs:  []string{},
			wantRoots: 1,
		},
		{
			name:      "unknown guid ignored",
			selected:  []string{"does-not-exist"},
			wantSubs:  []string{},
			wantDirs:  []string{},
			wantRoots: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHierarchy(referenceHierarchy())
			sel := h.Expand(tt.selected)
			require.Equal(t, tt.wantSubs, sorted(sel.SubaccountIDs()))
			require.Equal(t, tt.wantDirs, sorted(sel.DirectoryIDs()))
		})
	}
}
