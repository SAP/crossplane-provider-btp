package directory

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/hierarchy"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

const KindName = "directory"

var (
	directoryCache resources.ResourceCache[*directory]
	directoryParam = configparam.StringSlice(KindName, "BTP directory ID or regex expression for name.").
			WithFlagName(KindName)
)

func init() {
	resources.RegisterKind(exporter{})
}

// Reset clears the memoized directory cache. Intended for tests.
func Reset() {
	directoryCache = nil
}

type exporter struct{}

var _ resources.Kind = exporter{}

// Param exposes the directory selector so tests (and other callers) can bind it
// without constructing an exporter.
func Param() configparam.ConfigParam {
	return directoryParam
}

func (e exporter) Param() configparam.ConfigParam {
	// Directories are derived from the selected subaccounts (the single export
	// entrypoint), but the selector lets the user narrow that derived set to the
	// matching ancestor directories. It never prompts; see Get.
	return directoryParam
}

func (e exporter) KindName() string {
	return KindName
}

func (e exporter) Export(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, options resources.Options) error {
	cache, err := Get(ctx, btpClient)
	if err != nil {
		return fmt.Errorf("failed to get cache with directories: %w", err)
	}
	slog.DebugContext(ctx, "Directories in cache after user selection", "count", cache.Len())

	if cache.Len() == 0 {
		eventHandler.Warn(fmt.Errorf("no directories found"))
	} else {
		for _, d := range cache.All() {
			eventHandler.Resource(convertDirectoryResource(ctx, btpClient, d, eventHandler, options.ResolveReferences))
		}
	}

	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*directory], error) {
	if directoryCache != nil {
		return directoryCache, nil
	}

	// Resolve the selected subaccounts (prompts once, shared with every exporter).
	saCache, err := subaccount.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve subaccount selection: %w", err)
	}

	// Derive the ancestor directories of the selected subaccounts by walking the
	// global account hierarchy upward.
	h, err := hierarchy.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get global account hierarchy: %w", err)
	}
	ancestors := h.AncestorDirectoriesForSubaccounts(saCache.AllIDs())
	slog.DebugContext(ctx, "Directories derived from selected subaccounts", "count", len(ancestors))

	// Build the cache from the derived ancestor directories.
	cache := resources.NewResourceCache[*directory]()
	for _, c := range ancestors {
		child := c
		cache.Set(&directory{
			Child:               &child,
			ResourceWithComment: yaml.NewResourceWithComment(nil),
		})
	}

	// Optionally narrow the derived ancestors to the ones the user selected via
	// --directory (matched by GUID, display value, or display-name regex). The
	// selector never prompts: directories are not an export entrypoint, so when
	// the flag is unset the full derived ancestor set is kept.
	directoryParam.WithPossibleValuesFn(func() ([]string, error) {
		return cache.ValuesForSelection().Values(), nil
	})
	if selected := directoryParam.Value(); len(selected) > 0 {
		cache.KeepSelectedOnly(selected)
		slog.DebugContext(ctx, "Directories after --directory filter", "count", cache.Len())
	}

	// Fetch the current administrators for each selected directory that has the user
	// authorization management (AUTHORIZATIONS) feature enabled. Directories without it
	// have no "Directory Administrator" role collection, so there are no admins to fetch;
	// convert will comment those out. A lookup failure for a single directory must not
	// abort the whole export.
	for _, d := range cache.All() {
		if !hasAuthorizations(d.DirectoryFeatures) {
			continue
		}
		admins, err := btpClient.GetDirectoryAdmins(ctx, d.GUID)
		if err != nil {
			slog.WarnContext(ctx, "Failed to get directory admins", "directory", d.GUID, "error", err)
			continue
		}
		d.Admins = admins
	}

	directoryCache = cache

	return directoryCache, nil
}

// hasAuthorizations reports whether the directory has the user authorization management
// feature enabled, which is a prerequisite for the "Directory Administrator" role
// collection to exist.
func hasAuthorizations(features []string) bool {
	for _, f := range features {
		if f == "AUTHORIZATIONS" {
			return true
		}
	}
	return false
}

func GetK8sResourceNameByID(ctx context.Context, btpClient *btpcli.BtpCli, id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("directory ID is not set")
	}

	dirCache, err := Get(ctx, btpClient)
	if err != nil {
		return "", fmt.Errorf("failed to get cache with directories: %w", err)
	}

	d := dirCache.Get(id)
	if d == nil {
		return "", fmt.Errorf("directory not found by ID: %s", id)
	}

	return d.GenerateK8sResourceName(), nil
}

type directory struct {
	*btpcli.Child
	*yaml.ResourceWithComment

	// Admins holds the user emails assigned to the "Directory Administrator" role
	// collection. Populated only for directories with the AUTHORIZATIONS feature; nil
	// otherwise. convert emits an empty list and comments the resource out when fewer
	// than two admins are available.
	Admins []string
}

var _ resources.BtpResource = &directory{}

func (d *directory) GetID() string {
	return d.GUID
}

func (d *directory) GetDisplayName() string {
	return d.DisplayName
}

func (d *directory) GetExternalName() string {
	return d.GUID
}

func (d *directory) GenerateK8sResourceName() string {
	return hierarchy.K8sResourceName(d.GetID(), d.GetDisplayName())
}
