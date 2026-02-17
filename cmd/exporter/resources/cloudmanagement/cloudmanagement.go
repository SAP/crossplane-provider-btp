package cloudmanagement

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
	cmCache resources.ResourceCache[*serviceinstancebase.ServiceInstance]
)

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*serviceinstancebase.ServiceInstance], error) {
	if cmCache != nil {
		return cmCache, nil
	}

	// Get complete list of service instances.
	siCache, err := serviceinstancebase.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve service instance cache: %w", err)
	}
	slog.DebugContext(ctx, "Service instances in cache before looking for cloud management", "count", siCache.Len())

	// Create service manager cache (only service instances that are service managers).
	cmCache = resources.NewResourceCache[*serviceinstancebase.ServiceInstance]()
	for _, si := range siCache.All() {
		if si.IsCloudManagement() {
			cmCache.Set(si)
		}
	}
	slog.DebugContext(ctx, "Found cloud management instances", "count", cmCache.Len())

	return cmCache, nil
}

func Convert(ctx context.Context, btpClient *btpcli.BtpCli, si *serviceinstancebase.ServiceInstance, eventHandler export.EventHandler, resolveReferences bool) {
	eventHandler.Resource(convertCloudManagementResource(ctx, btpClient, si, eventHandler, resolveReferences))
}
