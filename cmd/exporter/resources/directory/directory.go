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
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/scope"
)

const KindName = "directory"

var (
	directoryCache resources.ResourceCache[*directory]
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

func (e exporter) Param() configparam.ConfigParam {
	// The scope package owns the combined subaccount/directory selector; the
	// directory kind does not register its own selector flag.
	return nil
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

	// Resolve the combined subaccount/directory scope selection (prompts once).
	selection, err := scope.Select(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve scope selection: %w", err)
	}
	slog.DebugContext(ctx, "Directories selected via scope", "count", len(selection.Directories))

	// Build the cache from the selected hierarchy directories.
	cache := resources.NewResourceCache[*directory]()
	for _, c := range selection.Directories {
		child := c
		cache.Set(&directory{
			Child:               &child,
			ResourceWithComment: yaml.NewResourceWithComment(nil),
		})
	}
	directoryCache = cache

	return directoryCache, nil
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
	if d.GetDisplayName() == "" {
		return resources.UndefinedName
	}

	rn, err := resources.GenerateK8sResourceName("", fmt.Sprintf("%s", d.GetDisplayName()))
	if err != nil {
		d.AddComment(fmt.Sprintf("cannot generate directory resource name: %s", err))
	}

	return rn
}
