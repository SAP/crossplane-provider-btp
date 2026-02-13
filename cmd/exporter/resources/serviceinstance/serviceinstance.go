package serviceinstance

import (
	"context"
	"fmt"
	"log/slog"
	"maps"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/servicemanager"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

const (
	KindName = "serviceinstance"
)

var (
	instanceCache resources.ResourceCache[*servicemanager.ServiceInstance]
	instanceParam = configparam.StringSlice(KindName, "Service instance ID or regex expression for name.").
		WithFlagName(KindName)
)

func init() {
	resources.RegisterKind(exporter{})
}

type exporter struct{}

var _ resources.Kind = exporter{}

func (e exporter) Param() configparam.ConfigParam {
	return instanceParam
}

func (e exporter) KindName() string {
	return KindName
}

func (e exporter) Export(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, resolveReferences bool) error {
	cache, err := Get(ctx, btpClient)
	if err != nil {
		return fmt.Errorf("failed to get cache with service instances: %w", err)
	}
	slog.DebugContext(ctx, "Service instances in cache after user selection", "count", cache.Len())

	if cache.Len() == 0 {
		eventHandler.Warn(fmt.Errorf("no service instances found"))
	} else {
		convert(ctx, btpClient, eventHandler, resolveReferences)
	}

	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*servicemanager.ServiceInstance], error) {
	if instanceCache != nil {
		return instanceCache, nil
	}

	// Let the user select relevant subaccounts.
	saCache, err := subaccount.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve subaccount cache: %w", err)
	}
	slog.DebugContext(ctx, "Subaccounts in cache after user selection", "count", saCache.Len())

	// Retrieve all service instances from selected subaccounts.
	var btpInstances []btpcli.ServiceInstance
	for _, saId := range saCache.AllIDs() {
		instances, err := btpClient.ListServiceInstances(ctx, saId)
		if err != nil {
			return nil, fmt.Errorf("failed to get service instances for subaccount %s: %w", saId, err)
		}
		btpInstances = append(btpInstances, instances...)
	}
	slog.DebugContext(ctx, "Total service instances returned by BTP CLI", "count", len(btpInstances))

	// Wrap service instances for internal processing and caching.
	instances := make([]*servicemanager.ServiceInstance, len(btpInstances))
	for i, si := range btpInstances {
		instances[i] = &servicemanager.ServiceInstance{
			ServiceInstance:     &si,
			ResourceWithComment: yaml.NewResourceWithComment(nil),
		}
	}

	// Create a cache and store all service instances.
	cache := resources.NewResourceCache[*servicemanager.ServiceInstance]()
	cache.Store(instances...)

	// Let the user select service instances that have to be exported.
	widgetValues := cache.ValuesForSelection()
	instanceParam.WithPossibleValuesFn(func() ([]string, error) {
		return widgetValues.Values(), nil
	})

	selectedInstances, err := instanceParam.ValueOrAsk(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter value: %s, %w", instanceParam.GetName(), err)
	}
	slog.DebugContext(ctx, "Selected service instances", "instances", selectedInstances)

	// Keep only selected service instances in the cache.
	cache.KeepSelectedOnly(selectedInstances)
	instanceCache = cache

	return instanceCache, nil
}

func convert(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, resolveReferences bool) {
	cache, err := Get(ctx, btpClient)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get cache with service instances", "error", err)
		return
	}

	// Collect more required metadata.
	for _, si := range cache.All() {
		err := servicemanager.AddServiceAndBindingInfo(ctx, btpClient, si)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to get service and binding data for instance", "id", si.GetID(), "error", err)
		}

		err = servicemanager.AddServiceManagerResourceName(ctx, btpClient, si)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to set service manager name for service instance", "id", si.GetID(), "error", err)
		}
	}

	// Export service instances that are not service manager. Note, which subaccounts are involved.
	subaccounts := make(map[string]bool)
	for _, si := range cache.All() {
		if !si.IsServiceManager() {
			eventHandler.Resource(convertServiceInstanceResource(ctx, btpClient, si, eventHandler, resolveReferences))
			subaccounts[si.SubaccountID] = true
		}
	}

	// Export selected service managers as well and take a note of their subaccounts.
	subaccountsWithSM := make(map[string]bool)
	for _, si := range cache.All() {
		if si.IsServiceManager() {
			servicemanager.Convert(ctx, btpClient, si, eventHandler, resolveReferences)
			subaccountsWithSM[si.SubaccountID] = true
		}
	}

	// Because service instance resources cannot be applied to the cluster without an accompanying service manager resource,
	// we export those service manager resources in addition, even if they we not explicitly selected by the user,
	// or even if the don't physically exist in BTP yet.
	for saID := range maps.Keys(subaccounts) {
		if !subaccountsWithSM[saID] {
			err := servicemanager.EnsureExportForSubaccount(ctx, btpClient, saID, eventHandler, resolveReferences)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to export service manager for subaccount", "subaccount ID", saID)
			}
		}
	}
}
