package servicebinding

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/servicebindingbase"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceinstance"
)

const (
	KindName = "servicebinding"
)

var (
	selectedCache resources.ResourceCache[*servicebindingbase.ServiceBinding]

	bindingParam = configparam.StringSlice(KindName, "Service binding ID or regex expression for name.").
		WithFlagName(KindName)
)

func init() {
	resources.RegisterKind(exporter{})
	export.AddConfigParams(bindingParam)
}

type exporter struct{}

var _ resources.Kind = exporter{}

func (e exporter) Param() configparam.ConfigParam {
	return nil
}

func (e exporter) KindName() string {
	return KindName
}

func (e exporter) Export(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, resolveReferences bool) error {
	cache, err := Get(ctx, btpClient)
	if err != nil {
		return fmt.Errorf("failed to get cache with service bindings: %w", err)
	}
	slog.DebugContext(ctx, "Service bindings in cache after user selection", "count", cache.Len())

	if cache.Len() == 0 {
		eventHandler.Warn(fmt.Errorf("no service bindings found"))
	} else {
		for _, e := range cache.All() {
			convert(ctx, btpClient, e, eventHandler, resolveReferences)
		}
	}

	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*servicebindingbase.ServiceBinding], error) {
	if selectedCache != nil {
		return selectedCache, nil
	}

	fc, err := servicebindingbase.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get full cache with service bindings: %w", err)
	}
	slog.DebugContext(ctx, "Service bindings in full cache before selection", "count", fc.Len())

	// Create a shallow copy of the full cache to keep only selected bindings,
	// so that the full cache remains unchanged for other resources that might need it during their export.
	cache := fc.Copy()

	// Let the user select service bindings to export.
	widgetValues := cache.ValuesForSelection()
	bindingParam.WithPossibleValuesFn(func() ([]string, error) {
		return widgetValues.Values(), nil
	})

	selectedBindings, err := bindingParam.ValueOrAsk(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter value: %s, %w", bindingParam.GetName(), err)
	}
	slog.DebugContext(ctx, "Selected service bindings", "bindings", selectedBindings)

	// Keep only selected bindings in the cache.
	cache.KeepSelectedOnly(selectedBindings)
	selectedCache = cache

	return selectedCache, nil
}

func convert(ctx context.Context, btpClient *btpcli.BtpCli, sb *servicebindingbase.ServiceBinding, eventHandler export.EventHandler, resolveReferences bool) {
	exportPrerequisiteResources(ctx, btpClient, sb, eventHandler, resolveReferences)
	eventHandler.Resource(convertServiceBindingResource(ctx, btpClient, sb, eventHandler, resolveReferences))
}

func exportPrerequisiteResources(ctx context.Context, btpClient *btpcli.BtpCli, sb *servicebindingbase.ServiceBinding, eventHandler export.EventHandler, resolveReferences bool) {
	exportServiceInstance(ctx, btpClient, sb, eventHandler, resolveReferences)
}

func exportServiceInstance(ctx context.Context, btpClient *btpcli.BtpCli, sb *servicebindingbase.ServiceBinding, eventHandler export.EventHandler, resolveReferences bool) {
	siID := sb.ServiceInstanceID
	siName, err := serviceinstance.ExportInstance(ctx, btpClient, siID, eventHandler, resolveReferences)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to export service instance", "id", siID, "error", err)
	}

	// Set Service Instance reference.
	if siName != "" {
		sb.ServiceInstanceName = siName
	}
}
