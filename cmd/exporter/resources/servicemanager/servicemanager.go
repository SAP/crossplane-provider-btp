package servicemanager

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli/export"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceinstancebase"
)

var (
	managerCache resources.ResourceCache[*serviceinstancebase.ServiceInstance]
)

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*serviceinstancebase.ServiceInstance], error) {
	if managerCache != nil {
		return managerCache, nil
	}

	// Get complete list of service instances.
	siCache, err := serviceinstancebase.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve service instance cache: %w", err)
	}
	slog.DebugContext(ctx, "Service instances in cache before looking for service managers", "count", siCache.Len())

	// Create service manager cache (only service instances that are service managers).
	managerCache = resources.NewResourceCache[*serviceinstancebase.ServiceInstance]()
	for _, si := range siCache.All() {
		if si.IsServiceManager() {
			managerCache.Set(si)
		}
	}
	slog.DebugContext(ctx, "Found service managers", "count", managerCache.Len())

	return managerCache, nil
}

func Convert(ctx context.Context, btpClient *btpcli.BtpCli, si *serviceinstancebase.ServiceInstance, eventHandler export.EventHandler, resolveReferences bool) {
	eventHandler.Resource(convertServiceManagerResource(ctx, btpClient, si, eventHandler, resolveReferences))
}

func EnsureExportForSubaccount(ctx context.Context, btpClient *btpcli.BtpCli, subaccountID string, eventHandler export.EventHandler, resolveReferences bool) error {
	sm, found, err := getServiceManager(ctx, btpClient, subaccountID)
	if err != nil {
		return fmt.Errorf("failed to retrieve service manager for subaccount %s: %w", subaccountID, err)
	}
	if found {
		eventHandler.Resource(convertServiceManagerResource(ctx, btpClient, sm, eventHandler, resolveReferences))
	} else {
		eventHandler.Resource(defaultServiceManagerResource(ctx, btpClient, subaccountID, eventHandler, resolveReferences))
	}

	return nil
}

func getServiceManager(ctx context.Context, btpClient *btpcli.BtpCli, subaccountID string) (*serviceinstancebase.ServiceInstance, bool, error) {
	cache, err := Get(ctx, btpClient)
	if err != nil {
		return nil, false, fmt.Errorf("failed to retrieve service manager cache: %w", err)
	}

	for _, id := range cache.AllIDs() {
		sm := cache.Get(id)
		if sm != nil && sm.SubaccountID == subaccountID && sm.Usable {
			return sm, true, nil
		}
	}

	return nil, false, nil
}
