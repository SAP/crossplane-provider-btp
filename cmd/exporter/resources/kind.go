package resources

import (
	"context"
	"maps"
	"slices"
	"sort"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
)

// Kind interface must be implemented by each BTP provider custom resource kind.
type Kind interface {
	KindName() string
	// Param method returns the configuration parameters specific
	// to a resource kind.
	Param() configparam.ConfigParam
	// Export method performs the export operation of a resource
	// kind. The method first identifies the resources that are to
	// be exported using the values of the related configuration
	// parameters. Then it collects the resource definitions
	// through BTP Client. Finally, the resources are exported
	// using the eventHandler.
	Export(ctx context.Context, btpClient *btpcli.BtpCli, evHandler export.EventHandler, options Options) error
}

var kinds = map[string]Kind{}

// RegisterKind function registers a resource kind.
func RegisterKind(kind Kind) {
	kinds[kind.KindName()] = kind
}

// ConfigParams returns the selector configuration parameter for every
// registered resource kind, ordered by kind name.
func ConfigParams() []configparam.ConfigParam {
	params := make([]configparam.ConfigParam, 0, len(kinds))
	for _, name := range KindNames() {
		if param := kinds[name].Param(); param != nil {
			params = append(params, param)
		}
	}
	return params
}

// KindNames function returns the names of the registered kinds.
func KindNames() []string {
	names := slices.Collect(maps.Keys(kinds))
	sort.Strings(names)
	return names
}

// ExportFn returns the export function of a given kind.
func ExportFn(kind string) func(context.Context, *btpcli.BtpCli, export.EventHandler, Options) error {
	resource, ok := kinds[kind]
	if !ok || resource == nil {
		return nil
	}
	return resource.Export
}
