package cfenvironment

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/cloudmanagement"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

const (
	KindName      = "cloudfoundry-environment"
	CfServiceName = "cloudfoundry"
)

var (
	cfEnvCache resources.ResourceCache[*CloudFoundryEnvironment]
	cfEnvParam = configparam.StringSlice(KindName, "CF environment ID or regex expression for name.").
		WithFlagName(KindName)
)

func init() {
	resources.RegisterKind(exporter{})
}

type exporter struct{}

var _ resources.Kind = exporter{}

func (e exporter) Param() configparam.ConfigParam {
	return cfEnvParam
}

func (e exporter) KindName() string {
	return KindName
}

func (e exporter) Export(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, resolveReferences bool) error {
	cache, err := Get(ctx, btpClient)
	if err != nil {
		return fmt.Errorf("failed to get cache with CF environment instances: %w", err)
	}
	slog.DebugContext(ctx, "CF environment instances in cache after user selection", "count", cache.Len())

	if cache.Len() == 0 {
		eventHandler.Warn(fmt.Errorf("no CF environment instances found"))
	} else {
		convert(ctx, btpClient, eventHandler, resolveReferences)
	}

	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*CloudFoundryEnvironment], error) {
	if cfEnvCache != nil {
		return cfEnvCache, nil
	}

	// Let the user select relevant subaccounts.
	saCache, err := subaccount.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve subaccount cache: %w", err)
	}
	slog.DebugContext(ctx, "Subaccounts in cache after user selection", "count", saCache.Len())

	// Retrieve all CF environment instances from selected subaccounts.
	var btpEnvironments []btpcli.EnvironmentInstance
	for _, saId := range saCache.AllIDs() {
		instances, err := btpClient.ListEnvironmentInstances(ctx, saId)
		if err != nil {
			return nil, fmt.Errorf("failed to get environment instances for subaccount %s: %w", saId, err)
		}
		btpEnvironments = append(btpEnvironments, instances...)
	}
	slog.DebugContext(ctx, "Total environments returned by BTP CLI", "count", len(btpEnvironments))

	// Wrap CF environment instances for internal processing and caching.
	// Filter out non-CF environments.
	instances := make([]*CloudFoundryEnvironment, 0, len(btpEnvironments))
	for _, e := range btpEnvironments {
		if !isCloudFoundryEnvironment(&e) {
			continue
		}
		instances = append(instances, &CloudFoundryEnvironment{
			EnvironmentInstance: &e,
			ResourceWithComment: yaml.NewResourceWithComment(nil),
		})
	}

	// Create a cache and store all CF environment instances.
	cache := resources.NewResourceCache[*CloudFoundryEnvironment]()
	cache.Store(instances...)

	// Let the user select CF environments to export.
	widgetValues := cache.ValuesForSelection()
	cfEnvParam.WithPossibleValuesFn(func() ([]string, error) {
		return widgetValues.Values(), nil
	})

	selectedEnvironments, err := cfEnvParam.ValueOrAsk(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter value: %s, %w", cfEnvParam.GetName(), err)
	}
	slog.DebugContext(ctx, "Selected CF environments", "environments", selectedEnvironments)

	// Keep only selected CF environments in the cache.
	cache.KeepSelectedOnly(selectedEnvironments)
	cfEnvCache = cache

	return cfEnvCache, nil
}

func isCloudFoundryEnvironment(instance *btpcli.EnvironmentInstance) bool {
	return instance.EnvironmentType == CfServiceName
}

func convert(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, resolveReferences bool) {
	cache, err := Get(ctx, btpClient)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get cache with cloud foundry environments", "error", err)
		return
	}

	for _, e := range cache.All() {
		exportPrerequisiteResources(ctx, btpClient, e, eventHandler, resolveReferences)
		eventHandler.Resource(convertCloudFoundryEnvResource(ctx, btpClient, e, eventHandler, resolveReferences))
	}
}

func exportPrerequisiteResources(ctx context.Context, btpClient *btpcli.BtpCli, e *CloudFoundryEnvironment, eventHandler export.EventHandler, resolveReferences bool) {
	// Export subaccount Cloud Management resource.
	cmName, err := cloudmanagement.ExportInstanceForSubaccount(ctx, btpClient, e.SubaccountGUID, eventHandler, resolveReferences)
	if err != nil {
		slog.WarnContext(ctx, "Failed to export cloud management for subaccount", "subaccount", e.SubaccountGUID, "error", err)
	}

	// Set Cloud Management name in CF environment resource for reference.
	if cmName != "" {
		e.CloudManagementName = cmName
	}
}

type CloudFoundryEnvironment struct {
	*btpcli.EnvironmentInstance
	*yaml.ResourceWithComment
	CloudManagementName string
}

var _ resources.BtpResource = &CloudFoundryEnvironment{}

func (e *CloudFoundryEnvironment) GetID() string {
	return e.ID
}

func (e *CloudFoundryEnvironment) GetDisplayName() string {
	return e.Name
}

func (e *CloudFoundryEnvironment) GetExternalName() string {
	if e.GetID() == "" {
		return resources.UndefinedExternalName
	}

	return e.GetID()
}

func (e *CloudFoundryEnvironment) GenerateK8sResourceName() string {
	name := e.GetDisplayName()
	if name == "" || e.SubaccountGUID == "" {
		return resources.UndefinedName
	}

	resourceName := fmt.Sprintf("%s-%s", name, e.SubaccountGUID)

	resourceName, err := resources.GenerateK8sResourceName("", resourceName, "")
	if err != nil {
		e.AddComment(fmt.Sprintf("cannot generate CF environment resource name: %s", err))
	}

	return resourceName
}
