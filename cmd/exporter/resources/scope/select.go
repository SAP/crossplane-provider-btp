package scope

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"sync"

	"github.com/SAP/xp-clifford/cli/configparam"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
)

// ParamName is the flag/config name of the combined scope selector.
const ParamName = "subaccount"

// selectorParam is the single scope selector presented to the user. It lists
// subaccounts and directories together. The flag name stays "subaccount" for
// backwards compatibility with existing invocations and configuration.
var selectorParam = configparam.StringSlice(ParamName, "BTP subaccount/directory ID or regex expression for name.").
	WithFlagName(ParamName)

// Param exposes the scope selector so main.go can register it as a config param.
func Param() configparam.ConfigParam {
	return selectorParam
}

var (
	mu       sync.Mutex
	resolved *Selection
)

// Reset clears the memoized selection. Intended for tests.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	resolved = nil
}

// Select resolves the scope selection exactly once and caches the result.
// Subsequent calls return the cached Selection, so the user is prompted only
// once regardless of which resource exporter triggers selection first.
func Select(ctx context.Context, btpClient *btpcli.BtpCli) (*Selection, error) {
	mu.Lock()
	defer mu.Unlock()

	if resolved != nil {
		return resolved, nil
	}

	ga, err := btpClient.GetGlobalAccountHierarchy(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get global account hierarchy: %w", err)
	}
	h := NewHierarchy(ga)

	// Build the combined list of selectable values (subaccounts + directories),
	// mapping each display string back to its GUID.
	displayToGUID := map[string]string{}
	for _, sa := range h.AllSubaccounts() {
		displayToGUID[displayValue(sa.DisplayName, sa.Guid)] = sa.Guid
	}
	for _, d := range h.AllDirectories() {
		displayToGUID[displayValue(d.DisplayName, d.GUID)] = d.GUID
	}

	possibleValues := make([]string, 0, len(displayToGUID))
	for dv := range displayToGUID {
		possibleValues = append(possibleValues, dv)
	}
	sort.Strings(possibleValues)

	selectorParam.WithPossibleValuesFn(func() ([]string, error) {
		return possibleValues, nil
	})

	selectedRaw, err := selectorParam.ValueOrAsk(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get scope selection: %w", err)
	}

	// Resolve the raw selection (display values, GUIDs, or regexes over display
	// names) into the concrete set of selected GUIDs, then expand directories.
	selectedGUIDs := resolveSelection(selectedRaw, displayToGUID, h)
	sel := h.Expand(selectedGUIDs)
	sel.rootCount = len(selectedGUIDs)
	resolved = &sel

	return resolved, nil
}

// RootCount returns the number of distinct top-level entities the user selected
// (subaccounts and/or directories), before directory expansion. A directory
// that expands to many subaccounts counts as a single root. Returns 0 if no
// selection has been resolved yet.
func RootCount(ctx context.Context, btpClient *btpcli.BtpCli) (int, error) {
	sel, err := Select(ctx, btpClient)
	if err != nil {
		return 0, err
	}
	return sel.rootCount, nil
}

// displayValue formats a selectable entry the same way the resource cache does,
// so GUIDs, display values and regexes behave consistently across the tool.
func displayValue(displayName, guid string) string {
	return fmt.Sprintf("%s - [%s]", displayName, guid)
}

// resolveSelection turns the raw user selection into a deduplicated set of
// GUIDs. A raw value may be a display value, a GUID, or a regex matching a
// display name. Mirrors resourceCache.KeepSelectedOnly resolution semantics.
func resolveSelection(selected []string, displayToGUID map[string]string, h *Hierarchy) []string {
	keep := map[string]struct{}{}

	// Known GUIDs for direct-ID matching.
	isKnownGUID := func(guid string) bool {
		if _, ok := h.Subaccount(guid); ok {
			return true
		}
		_, ok := h.Directory(guid)
		return ok
	}

	var nameRxs []*regexp.Regexp
	for _, v := range selected {
		if guid, ok := displayToGUID[v]; ok {
			keep[guid] = struct{}{}
		} else if isKnownGUID(v) {
			keep[v] = struct{}{}
		} else if rx, err := regexp.Compile(v); err == nil {
			nameRxs = append(nameRxs, rx)
		}
	}

	// Match display names by regex.
	if len(nameRxs) > 0 {
		match := func(name, guid string) {
			for _, rx := range nameRxs {
				if rx.MatchString(name) {
					keep[guid] = struct{}{}
					return
				}
			}
		}
		for _, sa := range h.AllSubaccounts() {
			match(sa.DisplayName, sa.Guid)
		}
		for _, d := range h.AllDirectories() {
			match(d.DisplayName, d.GUID)
		}
	}

	guids := make([]string, 0, len(keep))
	for guid := range keep {
		guids = append(guids, guid)
	}
	return guids
}
