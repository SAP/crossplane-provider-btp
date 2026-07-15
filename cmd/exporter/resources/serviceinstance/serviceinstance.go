package serviceinstance

import (
	"context"
	"fmt"
	"log/slog"

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
	registry = resources.NewRegistry()
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
		return nil
	}

	for _, si := range cache.All() {
		convert(ctx, btpClient, si, eventHandler, resolveReferences)
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

func ExportInstance(ctx context.Context, btpClient *btpcli.BtpCli, instanceID string, eventHandler export.EventHandler, resolveReferences bool) (string, error) {
	si, err := getServiceInstance(ctx, btpClient, instanceID)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve service instance %s: %w", instanceID, err)
	}

	convert(ctx, btpClient, si, eventHandler, resolveReferences)

	return si.GenerateK8sResourceName(), nil
}

func getServiceInstance(ctx context.Context, btpClient *btpcli.BtpCli, instanceID string) (*serviceinstancebase.ServiceInstance, error) {
	// Get complete list of service instances.
	cache, err := serviceinstancebase.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve service instance cache: %w", err)
	}

	si := cache.Get(instanceID)
	if si == nil {
		return nil, fmt.Errorf("service instance with ID %q not found in cache", instanceID)
	}

	return si, nil
}

func convert(ctx context.Context, btpClient *btpcli.BtpCli, si *serviceinstancebase.ServiceInstance, eventHandler export.EventHandler, resolveReferences bool) {
	// Instances of certain services, e.g. Service Manager, Cloud Management or XSUAA, require special handling.
	switch {
	case si.IsCloudManagement():
		cloudmanagement.Convert(ctx, btpClient, si, eventHandler, resolveReferences)
	case si.IsServiceManager():
		servicemanager.Convert(ctx, btpClient, si, eventHandler, resolveReferences)
	default:
		if register(ctx, si) {
			exportPrerequisiteResources(ctx, btpClient, si, eventHandler, resolveReferences)
			eventHandler.Resource(convertServiceInstanceResource(ctx, btpClient, si, eventHandler, resolveReferences))
		}
	}
}

func register(ctx context.Context, si *serviceinstancebase.ServiceInstance) bool {
	success := registry.Register(si.GetID())
	if !success {
		slog.DebugContext(ctx, "Service instance already exported", "id", si.GetID())
	}
	return success
}

func exportPrerequisiteResources(ctx context.Context, btpClient *btpcli.BtpCli, si *serviceinstancebase.ServiceInstance, eventHandler export.EventHandler, resolveReferences bool) {
	// Export subaccount service manager.
	saID := si.SubaccountID
	smName, err := servicemanager.ExportOperatorInstance(ctx, btpClient, saID, eventHandler, resolveReferences)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to export service manager for subaccount", "subaccount ID", saID)
	}

	// Set Service Manager reference.
	if smName != "" {
		si.ServiceManagerName = smName
	}
}
