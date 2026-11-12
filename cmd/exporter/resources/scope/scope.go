// Package scope builds a navigable view of the global account hierarchy
// (directories and subaccounts) and expands a user selection into the full,
// deduplicated set of subaccounts and directories to export.
//
// Selecting a directory recursively includes every descendant directory and
// subaccount. Selecting a subaccount (or a directory) that is also reachable
// through another selected directory contributes it only once.
package scope

import (
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
)

// Hierarchy is a navigable view of the global account hierarchy.
type Hierarchy struct {
	// directories holds every directory node keyed by its GUID.
	directories map[string]btpcli.Child
	// subaccounts holds every subaccount node keyed by its GUID.
	subaccounts map[string]btpcli.HierarchySubaccount
	// childDirs maps a directory GUID to the GUIDs of its direct child directories.
	childDirs map[string][]string
	// childSubaccounts maps a directory GUID to the GUIDs of its direct subaccounts.
	childSubaccounts map[string][]string
}

// NewHierarchy flattens the global account hierarchy returned by the BTP CLI
// into lookup maps and a parent -> children index.
func NewHierarchy(ga btpcli.GlobalaccountHierarchy) *Hierarchy {
	h := &Hierarchy{
		directories:      make(map[string]btpcli.Child),
		subaccounts:      make(map[string]btpcli.HierarchySubaccount),
		childDirs:        make(map[string][]string),
		childSubaccounts: make(map[string][]string),
	}

	// Subaccounts parented directly by the global account.
	for _, sa := range ga.Subaccounts {
		h.subaccounts[sa.Guid] = sa
	}

	// Walk the directory tree; each directory records its direct children.
	for i := range ga.Children {
		h.indexDirectory(ga.Children[i])
	}

	return h
}

// indexDirectory records a directory, its direct child directories and
// subaccounts, and recurses into nested directories.
func (h *Hierarchy) indexDirectory(d btpcli.Child) {
	h.directories[d.GUID] = d

	for _, sa := range d.Subaccounts {
		h.subaccounts[sa.Guid] = sa
		h.childSubaccounts[d.GUID] = append(h.childSubaccounts[d.GUID], sa.Guid)
	}

	for i := range d.Children {
		child := d.Children[i]
		h.childDirs[d.GUID] = append(h.childDirs[d.GUID], child.GUID)
		h.indexDirectory(child)
	}
}

// AllDirectories returns every directory node in the hierarchy.
func (h *Hierarchy) AllDirectories() []btpcli.Child {
	out := make([]btpcli.Child, 0, len(h.directories))
	for _, d := range h.directories {
		out = append(out, d)
	}
	return out
}

// AllSubaccounts returns every subaccount node in the hierarchy.
func (h *Hierarchy) AllSubaccounts() []btpcli.HierarchySubaccount {
	out := make([]btpcli.HierarchySubaccount, 0, len(h.subaccounts))
	for _, sa := range h.subaccounts {
		out = append(out, sa)
	}
	return out
}

// Directory returns the directory node for a GUID, if present.
func (h *Hierarchy) Directory(guid string) (btpcli.Child, bool) {
	d, ok := h.directories[guid]
	return d, ok
}

// Subaccount returns the subaccount node for a GUID, if present.
func (h *Hierarchy) Subaccount(guid string) (btpcli.HierarchySubaccount, bool) {
	sa, ok := h.subaccounts[guid]
	return sa, ok
}

// Selection is the resolved, deduplicated set of subaccounts and directories to
// export, keyed by GUID.
type Selection struct {
	Subaccounts map[string]btpcli.HierarchySubaccount
	Directories map[string]btpcli.Child

	// rootCount is the number of distinct top-level entities the user selected
	// (subaccounts and/or directories) before directory expansion.
	rootCount int
}

// SubaccountIDs returns the GUIDs of the selected subaccounts.
func (s Selection) SubaccountIDs() []string {
	ids := make([]string, 0, len(s.Subaccounts))
	for id := range s.Subaccounts {
		ids = append(ids, id)
	}
	return ids
}

// DirectoryIDs returns the GUIDs of the selected directories.
func (s Selection) DirectoryIDs() []string {
	ids := make([]string, 0, len(s.Directories))
	for id := range s.Directories {
		ids = append(ids, id)
	}
	return ids
}

// Expand resolves a set of user-selected GUIDs (which may be subaccounts or
// directories, in any mix) into the full deduplicated selection. Every selected
// directory contributes all of its descendant directories and subaccounts,
// recursively. GUIDs that match neither a known directory nor subaccount are
// ignored (callers resolve regex/name selectors before calling Expand).
func (h *Hierarchy) Expand(selectedGUIDs []string) Selection {
	sel := Selection{
		Subaccounts: make(map[string]btpcli.HierarchySubaccount),
		Directories: make(map[string]btpcli.Child),
	}

	for _, guid := range selectedGUIDs {
		if sa, ok := h.subaccounts[guid]; ok {
			sel.Subaccounts[guid] = sa
			continue
		}
		if _, ok := h.directories[guid]; ok {
			h.collectDirectory(guid, sel)
		}
	}

	return sel
}

// collectDirectory adds a directory, all of its descendant directories, and all
// descendant subaccounts to the selection. The GUID-keyed maps make the
// operation idempotent, so overlapping selections deduplicate automatically.
func (h *Hierarchy) collectDirectory(guid string, sel Selection) {
	d, ok := h.directories[guid]
	if !ok {
		return
	}
	sel.Directories[guid] = d

	for _, saGUID := range h.childSubaccounts[guid] {
		if sa, ok := h.subaccounts[saGUID]; ok {
			sel.Subaccounts[saGUID] = sa
		}
	}

	for _, childGUID := range h.childDirs[guid] {
		h.collectDirectory(childGUID, sel)
	}
}
