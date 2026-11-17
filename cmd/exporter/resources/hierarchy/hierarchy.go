// Package hierarchy builds a navigable view of the global account hierarchy
// (directories and the subaccounts contained in each level) and derives, for a
// given subaccount, the chain of ancestor directories above it.
//
// The subaccount is the single entrypoint for an export. Directories are not
// selected directly; they are derived by walking upward from each selected
// subaccount to the global account, collecting every directory on the way.
package hierarchy

import (
	"context"
	"fmt"
	"sync"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
)

var (
	mu     sync.Mutex
	cached *Hierarchy
)

// Reset clears the memoized hierarchy. Intended for tests.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	cached = nil
}

// Get retrieves the global account hierarchy exactly once and caches the
// result, so every consumer (subaccount ancestry, directory derivation,
// reference resolution) shares a single CLI call.
func Get(ctx context.Context, btpClient *btpcli.BtpCli) (*Hierarchy, error) {
	mu.Lock()
	defer mu.Unlock()

	if cached != nil {
		return cached, nil
	}

	ga, err := btpClient.GetGlobalAccountHierarchy(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get global account hierarchy: %w", err)
	}
	cached = New(ga)

	return cached, nil
}

// Hierarchy is a navigable view of the global account hierarchy that supports
// walking upward from a subaccount through its ancestor directories.
type Hierarchy struct {
	// globalAccountGUID is the GUID of the global account at the root.
	globalAccountGUID string
	// directories holds every directory node keyed by its GUID.
	directories map[string]btpcli.Child
	// dirParent maps a directory GUID to its direct parent GUID, derived from the
	// structural nesting of the hierarchy (authoritative; nested payload nodes do
	// not always populate parentGUID).
	dirParent map[string]string
	// subaccountParent maps a subaccount GUID to its direct parent GUID.
	subaccountParent map[string]string
}

// New flattens the global account hierarchy returned by the BTP CLI into lookup
// maps and a child -> parent index used for the upward ancestor walk.
func New(ga btpcli.GlobalaccountHierarchy) *Hierarchy {
	h := &Hierarchy{
		globalAccountGUID: ga.GUID,
		directories:       make(map[string]btpcli.Child),
		dirParent:         make(map[string]string),
		subaccountParent:  make(map[string]string),
	}

	// Subaccounts parented directly by the global account.
	for _, sa := range ga.Subaccounts {
		h.subaccountParent[sa.Guid] = ga.GUID
	}

	// Walk the directory tree; each directory records its structural parent and
	// the parent of every subaccount it contains.
	for i := range ga.Children {
		h.indexDirectory(ga.Children[i], ga.GUID)
	}

	return h
}

// indexDirectory records a directory, its structural parent, and the parent of
// each subaccount it contains, then recurses into nested directories.
func (h *Hierarchy) indexDirectory(d btpcli.Child, parent string) {
	guid := directoryGUID(d)
	h.directories[guid] = d
	h.dirParent[guid] = parent

	for _, sa := range d.Subaccounts {
		h.subaccountParent[sa.Guid] = guid
	}

	for i := range d.Children {
		h.indexDirectory(d.Children[i], guid)
	}
}

// Directory returns the directory node for a GUID, if present.
func (h *Hierarchy) Directory(guid string) (btpcli.Child, bool) {
	d, ok := h.directories[guid]
	return d, ok
}

// AncestorDirectories returns the directories on the path from the given
// subaccount up to (but excluding) the global account, nearest parent first.
// A subaccount parented directly by the global account has no ancestors.
func (h *Hierarchy) AncestorDirectories(subaccountGUID string) []btpcli.Child {
	var out []btpcli.Child

	guid, ok := h.subaccountParent[subaccountGUID]
	for ok && guid != "" && guid != h.globalAccountGUID {
		d, found := h.directories[guid]
		if !found {
			break
		}
		out = append(out, d)
		guid, ok = h.dirParent[guid]
	}

	return out
}

// AncestorDirectoriesForSubaccounts returns the deduplicated set of ancestor
// directories for the given subaccount GUIDs, keyed by directory GUID.
func (h *Hierarchy) AncestorDirectoriesForSubaccounts(subaccountGUIDs []string) map[string]btpcli.Child {
	result := make(map[string]btpcli.Child)
	for _, saGUID := range subaccountGUIDs {
		for _, d := range h.AncestorDirectories(saGUID) {
			result[directoryGUID(d)] = d
		}
	}
	return result
}

// DirectoryK8sResourceName returns the k8s resource name a directory would be
// exported under. It delegates to K8sResourceName, the single naming rule the
// directory exporter also uses, so references resolve to the exact name that is
// emitted. This does not require the directory cache and keeps the subaccount
// package free of a dependency on the directory package.
func (h *Hierarchy) DirectoryK8sResourceName(guid string) (string, error) {
	if guid == "" {
		return "", fmt.Errorf("directory ID is not set")
	}

	d, ok := h.directories[guid]
	if !ok {
		return "", fmt.Errorf("directory not found by ID: %s", guid)
	}

	return K8sResourceName(d.GUID, d.DisplayName), nil
}

func K8sResourceName(guid, displayName string) string {
	if displayName == "" {
		return resources.UndefinedName
	}

	rn, err := resources.GenerateK8sResourceName(guid, displayName)
	if err != nil {
		return resources.UndefinedName
	}

	return rn
}

// directoryGUID returns the directory's GUID.
func directoryGUID(d btpcli.Child) string {
	return d.GUID
}
