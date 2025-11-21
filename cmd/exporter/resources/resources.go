package resources

import (
	"context"

	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/configparam"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/export"
)

// Kind interface must be implemented by each BTP provider custom resource kind.
type Kind interface {
	// Param method returns the configuration parameters specific
	// to a resource kind.
	Param() configparam.ConfigParam
	// Export method performs the export operation of a resource
	// kind. The method first identifies the resources that are to
	// be exported using the values of the related configuration
	// parameters. Then it collects the resource definitions
	// through BTP Client. Finally, the resources are exported
	// using the eventHandler.
	Export(ctx context.Context, btpClient *btp.Client, evHandler export.EventHandler, resolveReferences bool) error
}

var kinds = map[string]Kind{}

// RegisterKind function registers a resource kind.
func RegisterKind(kind Kind) {
	kinds[kind.Param().GetName()] = kind
}

// ConfigParams function returns the configuration parameters of all
// registered resource kinds.
func ConfigParams() []configparam.ConfigParam {
	result := make([]configparam.ConfigParam, len(kinds))
	i := 0
	for _, kind := range kinds {
		result[i] = kind.Param()
		i++
	}
	return result
}

// ExportFn returns the export function of a given kind.
func ExportFn(kind string) func(context.Context, *btp.Client, export.EventHandler, bool) error {
	resource, ok := kinds[kind]
	if !ok || resource == nil {
		return nil
	}
	return resource.Export
}
