package serviceinstance

import (
	"context"
	"fmt"
	"log/slog"
	"maps"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/cloudmanagement"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceinstancebase"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/servicemanager"
)

const (
	KindName = "serviceinstance"
)

var (
	instanceCache resources.ResourceCache[*serviceinstancebase.ServiceInstance]
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

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*serviceinstancebase.ServiceInstance], error) {
	if instanceCache != nil {
		return instanceCache, nil
	}

	// Get complete list of service instances.
	siCache, err := serviceinstancebase.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve service instance cache: %w", err)
	}
	slog.DebugContext(ctx, "Service instances in cache before user selection", "count", siCache.Len())

	// Create service instance cache.
	cache := resources.NewResourceCache[*serviceinstancebase.ServiceInstance]()
	for _, si := range siCache.All() {
		cache.Set(si)
	}

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

	// Export service instances. Note involved subaccounts.
	subaccounts := make(map[string]bool)
	subaccountsWithSM := make(map[string]bool)

	for _, si := range cache.All() {
		// Make sure that the service instance is linked to a service manager instance.
		err = servicemanager.AddServiceManagerResourceName(ctx, btpClient, si)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to set service manager name for service instance", "id", si.GetID(), "error", err)
		}

		// Instances of certain services, e.g. Service Manager, Cloud Management or XSUAA, require special handling.
		switch {
		case si.IsCloudManagement():
			cloudmanagement.Convert(ctx, btpClient, si, eventHandler, resolveReferences)
			subaccounts[si.SubaccountID] = true
		case si.IsServiceManager():
			servicemanager.Convert(ctx, btpClient, si, eventHandler, resolveReferences)
			subaccountsWithSM[si.SubaccountID] = true
		default:
			eventHandler.Resource(convertServiceInstanceResource(ctx, btpClient, si, eventHandler, resolveReferences))
			subaccounts[si.SubaccountID] = true
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
