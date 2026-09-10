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
	registry      = resources.NewRegistry()

	bindingParam = configparam.StringSlice(KindName, "Service binding ID or regex expression for name.").
			WithFlagName(KindName)
)

func init() {
	resources.RegisterKind(exporter{})
}

type exporter struct{}

var _ resources.Kind = exporter{}

func (e exporter) Param() configparam.ConfigParam {
	return bindingParam
}

func (e exporter) KindName() string {
	return KindName
}

func (e exporter) Export(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, options resources.Options) error {
	cache, err := Get(ctx, btpClient, options)
	if err != nil {
		return fmt.Errorf("failed to get cache with service bindings: %w", err)
	}
	slog.DebugContext(ctx, "Service bindings in cache after user selection", "count", cache.Len())

	if cache.Len() == 0 {
		eventHandler.Warn(fmt.Errorf("no service bindings found"))
	} else {
		for _, e := range cache.All() {
			convert(ctx, btpClient, e, eventHandler, options.ResolveReferences)
		}
	}

	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli, options resources.Options) (resources.ResourceCache[*servicebindingbase.ServiceBinding], error) {
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

	// If the user has already selected service instances, restrict bindings to those instances only.
	if err := filterBySelectedInstances(ctx, btpClient, cache, options); err != nil {
		return nil, fmt.Errorf("failed to filter service bindings: %w", err)
	}

	if err := resources.SelectCache(ctx, cache, bindingParam, options); err != nil {
		return nil, err
	}
	selectedCache = cache

	return selectedCache, nil
}

func filterBySelectedInstances(ctx context.Context, btpClient *btpcli.BtpCli, cache resources.ResourceCache[*servicebindingbase.ServiceBinding], options resources.Options) error {
	siCache, err := serviceinstance.Get(ctx, btpClient, options)
	if err != nil {
		return fmt.Errorf("failed to get service instance cache: %w", err)
	}
	if siCache.Len() == 0 {
		return nil
	}

	// Build set of selected regular instance IDs, excluding ServiceManager and
	// CloudManagement instances whose bindings are managed internally by those CRs.
	selectedInstanceIDs := make(map[string]bool, siCache.Len())
	for _, id := range siCache.AllIDs() {
		si := siCache.Get(id)
		if si != nil && !si.IsServiceManager() && !si.IsCloudManagement() {
			selectedInstanceIDs[id] = true
		}
	}

	var bindingsToKeep []string
	for _, sb := range cache.All() {
		if selectedInstanceIDs[sb.ServiceInstanceID] {
			bindingsToKeep = append(bindingsToKeep, sb.GetID())
		}
	}

	cache.KeepSelectedOnly(bindingsToKeep)
	slog.DebugContext(ctx, "Service bindings after filtering by selected service instances", "count", cache.Len())

	return nil
}

func convert(ctx context.Context, btpClient *btpcli.BtpCli, sb *servicebindingbase.ServiceBinding, eventHandler export.EventHandler, resolveReferences bool) {
	if !register(ctx, sb) {
		return
	}

	exportPrerequisiteResources(ctx, btpClient, sb, eventHandler, resolveReferences)
	eventHandler.Resource(convertServiceBindingResource(ctx, btpClient, sb, eventHandler, resolveReferences))
}

func register(ctx context.Context, sb *servicebindingbase.ServiceBinding) bool {
	success := registry.Register(sb.GetID())
	if !success {
		slog.DebugContext(ctx, "Service binding already exported", "id", sb.GetID())
	}
	return success
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
		sb.ServiceInstanceK8sName = siName
	}
}
