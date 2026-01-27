package resources

import (
	"context"

	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/configparam"
	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/export"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/client"
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
	Export(ctx context.Context, btpClient *client.Client, evHandler export.EventHandler, resolveReferences bool) error
}

var kinds = map[string]Kind{}

// RegisterKind function registers a resource kind.
func RegisterKind(kind Kind) {
	kinds[kind.KindName()] = kind
}

// ConfigParams function returns the configuration parameters of all
// registered resource kinds.
func ConfigParams() []configparam.ConfigParam {
	result := make([]configparam.ConfigParam, 0, len(kinds))
	for _, kind := range kinds {
		if p := kind.Param(); p != nil {
			result = append(result, p)
		}
	}
	return result
}

// ExportFn returns the export function of a given kind.
func ExportFn(kind string) func(context.Context, *client.Client, export.EventHandler, bool) error {
	resource, ok := kinds[kind]
	if !ok || resource == nil {
		return nil
	}
	return resource.Export
}
