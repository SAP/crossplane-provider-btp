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

type exporter struct{}

var _ resources.Kind = exporter{}

func (e exporter) Param() configparam.ConfigParam {
	return directoryParam
}

func (e exporter) KindName() string {
	return directoryParam.GetName()
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

	// Retrieve the directory hierarchy of the global account.
	children, err := btpClient.ListGlobalAccountDirectories(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get directories: %w", err)
	}

	// Flatten the directory tree since the CLI returns nested children.
	var originals []*btpcli.Child
	flattenDirectories(children, &originals)
	slog.DebugContext(ctx, "Total directories returned by BTP CLI", "count", len(originals))

	// Wrap directories for internal processing and caching.
	var directories []*directory
	for _, c := range originals {
		directories = append(directories, &directory{
			Child:               c,
			ResourceWithComment: yaml.NewResourceWithComment(nil),
		})
	}

	// Create cache and store all directories.
	cache := resources.NewResourceCache[*directory]()
	cache.Store(directories...)

	// Let the user select directories to export.
	widgetValues := cache.ValuesForSelection()
	directoryParam.WithPossibleValuesFn(func() ([]string, error) {
		return widgetValues.Values(), nil
	})

	selectedDirectories, err := directoryParam.ValueOrAsk(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter value: %s, %w", directoryParam.GetName(), err)
	}
	slog.DebugContext(ctx, "Selected directories", "directories", selectedDirectories)

	// Keep only selected directories in the cache.
	cache.KeepSelectedOnly(selectedDirectories)
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

// flattenDirectories recursively collects all directory nodes from the nested
// hierarchy returned by the BTP CLI into a flat slice.
func flattenDirectories(children []btpcli.Child, out *[]*btpcli.Child) {
	for i := range children {
		c := children[i]
		// The hierarchy keeps subaccounts in a separate `subaccounts` field, so
		// everything in `children` is a directory. Guard on directoryType in case
		// Child is extended to also capture subaccount nodes.
		if c.DirectoryType == "" {
			continue
		}
		*out = append(*out, &c)
		flattenDirectories(c.Children, out)
	}
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

	// Format: <display-name>.<region>
	rn, err := resources.GenerateK8sResourceName("", fmt.Sprintf("%s", d.GetDisplayName()))
	if err != nil {
		d.AddComment(fmt.Sprintf("cannot generate directory resource name: %s", err))
	}

	return rn
}
