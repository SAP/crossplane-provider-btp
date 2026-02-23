package servicemanager

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/servicebinding"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceplan"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

const (
	OfferingServiceManager = "service-manager"
	DefaultNamePrefix      = "managed-service-manager"
	DefaultSecretNamespace = "default"
)

var (
	managerCache resources.ResourceCache[*ServiceInstance]
)

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*ServiceInstance], error) {
	if managerCache != nil {
		return managerCache, nil
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
	instances := make([]*ServiceInstance, len(btpInstances))
	for i, si := range btpInstances {
		instances[i] = &ServiceInstance{
			ServiceInstance:     &si,
			ResourceWithComment: yaml.NewResourceWithComment(nil),
		}
	}

	// Add additional metadata to service instances.
	for _, sm := range instances {
		if err := AddServiceAndBindingInfo(ctx, btpClient, sm); err != nil {
			slog.WarnContext(ctx, "Failed to preprocess service instance", "id", sm.ID, "error", err)
		}
	}

	// Filter out service instances that are not service managers.
	var managers []*ServiceInstance
	for _, sm := range instances {
		if sm.IsServiceManager() {
			managers = append(managers, sm)
		}
	}
	slog.DebugContext(ctx, "Found service managers", "count", len(managers))

	// Create a cache and store all service managers.
	managerCache = resources.NewResourceCache[*ServiceInstance]()
	managerCache.Store(managers...)

	return managerCache, nil
}

func AddServiceAndBindingInfo(ctx context.Context, btpClient *btpcli.BtpCli, si *ServiceInstance) error {
	// Service plans for selected subaccounts.
	planCache, err := serviceplan.Get(ctx, btpClient)
	if err != nil {
		return fmt.Errorf("failed to retrieve service plan cache: %w", err)
	}
	slog.DebugContext(ctx, "Service plans in cache", "count", planCache.Len())

	// Add service metadata to service instance.
	plan := planCache.Get(si.ServicePlanID)
	if plan == nil {
		slog.WarnContext(ctx, "Service plan not found", "id", si.ServicePlanID)
	} else {
		si.OfferingName = plan.ServiceOfferingName
		si.PlanName = plan.Name
	}

	// Add service binding ID.
	bindingId, found, err := servicebinding.GetBindingIdByServiceInstanceId(ctx, btpClient, si.ID)
	if err != nil {
		slog.WarnContext(ctx, "Failed to retrieve binding id from service instance", "id", si.ID)
	} else if found {
		si.BindingID = bindingId
	}

	return nil
}

func AddServiceManagerResourceName(ctx context.Context, btpClient *btpcli.BtpCli, si *ServiceInstance) error {
	name, err := getServiceManagerResourceName(ctx, btpClient, si.SubaccountID)
	if err != nil {
		return fmt.Errorf("failed to get service manager name for subaccount %s: %w", si.SubaccountID, err)
	}
	si.ServiceManagerName = name

	return nil
}

func Convert(ctx context.Context, btpClient *btpcli.BtpCli, si *ServiceInstance, eventHandler export.EventHandler, resolveReferences bool) {
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

func getServiceManager(ctx context.Context, btpClient *btpcli.BtpCli, subaccountID string) (*ServiceInstance, bool, error) {
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

func getServiceManagerResourceName(ctx context.Context, btpClient *btpcli.BtpCli, subaccountID string) (string, error) {
	sm, found, err := getServiceManager(ctx, btpClient, subaccountID)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve service manager for subaccount %s: %w", subaccountID, err)
	}
	if !found {
		return defaultServiceManagerResourceName(subaccountID), nil
	}

	return sm.GenerateK8sResourceName(), nil
}

func defaultServiceManagerResourceName(subaccountID string) string {
	if subaccountID == "" {
		return resources.UndefinedName
	}
	return fmt.Sprintf("%s-%s", DefaultNamePrefix, subaccountID)
}

type ServiceInstance struct {
	*btpcli.ServiceInstance
	*yaml.ResourceWithComment
	OfferingName       string
	PlanName           string
	BindingID          string
	ServiceManagerName string
}

var _ resources.BtpResource = &ServiceInstance{}

func (sm *ServiceInstance) GetID() string {
	return sm.ID
}

func (sm *ServiceInstance) GetDisplayName() string {
	return sm.Name
}

func (sm *ServiceInstance) GetExternalName() string {
	if sm.IsServiceManager() {
		return sm.serviceManagerExternalName()
	}
	return sm.serviceInstanceExternalName()
}

func (sm *ServiceInstance) serviceInstanceExternalName() string {
	if sm.GetID() == "" || sm.SubaccountID == "" {
		return resources.UndefinedExternalName
	}

	return fmt.Sprintf("%s,%s", sm.SubaccountID, sm.GetID())
}

func (sm *ServiceInstance) serviceManagerExternalName() string {
	if sm.GetID() == "" || sm.BindingID == "" {
		return resources.UndefinedExternalName
	}

	return fmt.Sprintf("%s/%s", sm.GetID(), sm.BindingID)
}

func (sm *ServiceInstance) GenerateK8sResourceName() string {
	name := sm.GetDisplayName()
	if name == "" || sm.SubaccountID == "" {
		return resources.UndefinedName
	}

	resourceName := fmt.Sprintf("%s-%s", name, sm.SubaccountID)

	resourceName, err := resources.GenerateK8sResourceName("", resourceName, "")
	if err != nil {
		sm.AddComment(fmt.Sprintf("cannot generate service manager resource name: %s", err))
	}

	return resourceName
}

func (sm *ServiceInstance) IsServiceManager() bool {
	return sm.OfferingName == OfferingServiceManager
}
