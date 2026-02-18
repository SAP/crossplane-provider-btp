package cloudmanagement

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceinstancebase"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/servicemanager"
)

const (
	defaultNamePrefix = "managed-cloud-management"
)

var (
	cmCache  resources.ResourceCache[*serviceinstancebase.ServiceInstance]
	registry = resources.NewRegistry()
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

func Convert(ctx context.Context, btpClient *btpcli.BtpCli, cm *serviceinstancebase.ServiceInstance, eventHandler export.EventHandler, resolveReferences bool) {
	if register(ctx, cm) {
		exportPrerequisiteResources(ctx, btpClient, cm, eventHandler, resolveReferences)
		eventHandler.Resource(convertCloudManagementResource(ctx, btpClient, cm, eventHandler, resolveReferences))
	}
}

func convertDefault(ctx context.Context, btpClient *btpcli.BtpCli, cm *serviceinstancebase.ServiceInstance, eventHandler export.EventHandler, resolveReferences bool) {
	exportPrerequisiteResources(ctx, btpClient, cm, eventHandler, resolveReferences)
	eventHandler.Resource(convertDefaultCloudManagementResource(ctx, btpClient, cm, eventHandler, resolveReferences))
}

func register(ctx context.Context, cm *serviceinstancebase.ServiceInstance) bool {
	success := registry.Register(cm.ID)
	if !success {
		slog.DebugContext(ctx, "Cloud management already exported", "subaccount", cm.SubaccountID, "instance", cm.GetID())
	}
	return success
}

func ExportInstanceForSubaccount(ctx context.Context, btpClient *btpcli.BtpCli, subaccountID string, eventHandler export.EventHandler, resolveReferences bool) (string, error) {
	cm, found, err := getCloudManagement(ctx, btpClient, subaccountID)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve cloud management instance for subaccount %s: %w", subaccountID, err)
	}

	if found {
		Convert(ctx, btpClient, cm, eventHandler, resolveReferences)
	} else {
		convertDefault(ctx, btpClient, cm, eventHandler, resolveReferences)
	}

	return cm.GenerateK8sResourceName(), nil
}

func getCloudManagement(ctx context.Context, btpClient *btpcli.BtpCli, subaccountID string) (*serviceinstancebase.ServiceInstance, bool, error) {
	cache, err := Get(ctx, btpClient)
	if err != nil {
		return nil, false, fmt.Errorf("failed to retrieve cloud management cache: %w", err)
	}

	for _, id := range cache.AllIDs() {
		cm := cache.Get(id)
		if cm != nil && cm.SubaccountID == subaccountID && cm.Usable {
			return cm, true, nil
		}
	}

	return defaultCloudManagement(subaccountID), false, nil
}

func exportPrerequisiteResources(ctx context.Context, btpClient *btpcli.BtpCli, cm *serviceinstancebase.ServiceInstance, eventHandler export.EventHandler, resolveReferences bool) {
	// Export subaccount service manager.
	saID := cm.SubaccountID
	smName, err := servicemanager.ExportInstanceForSubaccount(ctx, btpClient, saID, eventHandler, resolveReferences)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to export service manager for subaccount", "subaccount ID", saID)
	}

	// Set Service Manager reference.
	if smName != "" {
		cm.ServiceManagerName = smName
	}
}

func defaultCloudManagement(subaccountID string) *serviceinstancebase.ServiceInstance {
	return &serviceinstancebase.ServiceInstance{
		ServiceInstance: &btpcli.ServiceInstance{
			ID:           fmt.Sprintf("%s-%s", defaultNamePrefix, subaccountID),
			Name:         defaultNamePrefix,
			SubaccountID: subaccountID,
			Usable:       true,
		},
		ResourceWithComment: yaml.NewResourceWithComment(nil),
	}
}
