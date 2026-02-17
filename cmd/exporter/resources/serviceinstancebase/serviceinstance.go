package serviceinstancebase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/yaml"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/servicebinding"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceplan"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

const (
	ServiceManagerOffering  = "service-manager"
	CloudManagementOffering = "cis"
	CloudManagementPlan     = "local"
)

var (
	instanceCache resources.ResourceCache[*ServiceInstance]
)

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*ServiceInstance], error) {
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
	instances := make([]*ServiceInstance, len(btpInstances))
	for i, si := range btpInstances {
		instances[i] = &ServiceInstance{
			ServiceInstance:     &si,
			ResourceWithComment: yaml.NewResourceWithComment(nil),
		}
	}

	// Add additional metadata to service instances.
	for _, si := range instances {
		if err := addServiceAndBindingInfo(ctx, btpClient, si); err != nil {
			slog.WarnContext(ctx, "Failed to preprocess service instance", "id", si.ID, "error", err)
		}
	}

	// Create a cache and store all service instances.
	instanceCache = resources.NewResourceCache[*ServiceInstance]()
	instanceCache.Store(instances...)

	return instanceCache, nil
}

func addServiceAndBindingInfo(ctx context.Context, btpClient *btpcli.BtpCli, si *ServiceInstance) error {
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

type ServiceInstance struct {
	*btpcli.ServiceInstance
	*yaml.ResourceWithComment
	OfferingName       string
	PlanName           string
	BindingID          string
	ServiceManagerName string
}

var _ resources.BtpResource = &ServiceInstance{}

func (si *ServiceInstance) GetID() string {
	return si.ID
}

func (si *ServiceInstance) GetDisplayName() string {
	return si.Name
}

func (si *ServiceInstance) GetExternalName() string {
	switch {
	case si.IsCloudManagement():
		return si.cloudManagementExternalName()
	case si.IsServiceManager():
		return si.serviceManagerExternalName()
	}

	return si.serviceInstanceExternalName()
}

func (si *ServiceInstance) serviceInstanceExternalName() string {
	if si.GetID() == "" || si.SubaccountID == "" {
		return resources.UndefinedExternalName
	}

	return fmt.Sprintf("%s,%s", si.SubaccountID, si.GetID())
}

func (si *ServiceInstance) serviceManagerExternalName() string {
	if si.GetID() == "" || si.BindingID == "" {
		return resources.UndefinedExternalName
	}

	return fmt.Sprintf("%s/%s", si.GetID(), si.BindingID)
}

func (si *ServiceInstance) cloudManagementExternalName() string {
	// Same format as for service manager.
	return si.serviceManagerExternalName()
}

func (si *ServiceInstance) GenerateK8sResourceName() string {
	name := si.GetDisplayName()
	if name == "" || si.SubaccountID == "" {
		return resources.UndefinedName
	}

	resourceName := fmt.Sprintf("%s-%s", name, si.SubaccountID)

	resourceName, err := resources.GenerateK8sResourceName("", resourceName, "")
	if err != nil {
		si.AddComment(fmt.Sprintf("cannot generate resource name: %s", err))
	}

	return resourceName
}

func (si *ServiceInstance) IsServiceManager() bool {
	return si.OfferingName == ServiceManagerOffering
}

func (si *ServiceInstance) IsCloudManagement() bool {
	return si.OfferingName == CloudManagementOffering && si.PlanName == CloudManagementPlan
}
