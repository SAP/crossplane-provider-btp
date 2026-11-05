package kymaenvironment

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
	KindName        = "kyma-environment"
	KymaServiceName = "kyma"
)

var (
	cfEnvCache resources.ResourceCache[*KymaEnvironment]
	cfEnvParam = configparam.StringSlice(KindName, "Kyma environment ID or regex expression for name.").
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

func (e exporter) Export(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, options resources.Options) error {
	cache, err := Get(ctx, btpClient, options)
	if err != nil {
		return fmt.Errorf("failed to get cache with Kyma environment instances: %w", err)
	}
	slog.DebugContext(ctx, "Kyma environment instances in cache after user selection", "count", cache.Len())

	if cache.Len() == 0 {
		eventHandler.Warn(fmt.Errorf("no Kyma environment instances found"))
	} else {
		convert(ctx, btpClient, eventHandler, options)
	}

	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli, options resources.Options) (resources.ResourceCache[*KymaEnvironment], error) {
	if cfEnvCache != nil {
		return cfEnvCache, nil
	}

	// Let the user select relevant subaccounts.
	saCache, err := subaccount.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve subaccount cache: %w", err)
	}
	slog.DebugContext(ctx, "Subaccounts in cache after user selection", "count", saCache.Len())

	// Retrieve all Kyma environment instances from selected subaccounts.
	var btpEnvironments []btpcli.EnvironmentInstance
	for _, saId := range saCache.AllIDs() {
		instances, err := btpClient.ListEnvironmentInstances(ctx, saId)
		if err != nil {
			return nil, fmt.Errorf("failed to get environment instances for subaccount %s: %w", saId, err)
		}
		btpEnvironments = append(btpEnvironments, instances...)
	}
	slog.DebugContext(ctx, "Total environments returned by BTP CLI", "count", len(btpEnvironments))

	// Wrap Kyma environment instances for internal processing and caching.
	// Filter out non-Kyma environments.
	instances := make([]*KymaEnvironment, 0, len(btpEnvironments))
	for _, e := range btpEnvironments {
		if !isKymaEnvironment(&e) {
			continue
		}
		instances = append(instances, &KymaEnvironment{
			EnvironmentInstance: &e,
			ResourceWithComment: yaml.NewResourceWithComment(nil),
		})
	}

	// Create a cache and store all Kyma environment instances.
	cache := resources.NewResourceCache[*KymaEnvironment]()
	cache.Store(instances...)

	if err := resources.SelectCache(ctx, cache, cfEnvParam, options); err != nil {
		return nil, err
	}
	cfEnvCache = cache

	return cfEnvCache, nil
}

func isKymaEnvironment(instance *btpcli.EnvironmentInstance) bool {
	return instance.EnvironmentType == KymaServiceName
}

func convert(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, options resources.Options) {
	cache, err := Get(ctx, btpClient, options)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get cache with kyma environments", "error", err)
		return
	}

	for _, e := range cache.All() {
		exportPrerequisiteResources(ctx, btpClient, e, eventHandler, options.ResolveReferences)
		eventHandler.Resource(convertKymaEnvResource(ctx, btpClient, e, eventHandler, options.ResolveReferences))
	}
}

func exportPrerequisiteResources(ctx context.Context, btpClient *btpcli.BtpCli, e *KymaEnvironment, eventHandler export.EventHandler, resolveReferences bool) {
	// Export subaccount Cloud Management resource.
	cmName, err := cloudmanagement.ExportInstanceForSubaccount(ctx, btpClient, e.SubaccountGUID, eventHandler, resolveReferences)
	if err != nil {
		slog.WarnContext(ctx, "Failed to export cloud management for subaccount", "subaccount", e.SubaccountGUID, "error", err)
	}

	// Set Cloud Management name in Kyma environment resource for reference.
	if cmName != "" {
		e.CloudManagementName = cmName
	}
}

type KymaEnvironment struct {
	*btpcli.EnvironmentInstance
	*yaml.ResourceWithComment
	CloudManagementName string
}

var _ resources.BtpResource = &KymaEnvironment{}

func (e *KymaEnvironment) GetID() string {
	return e.ID
}

func (e *KymaEnvironment) GetDisplayName() string {
	return e.Name
}

func (e *KymaEnvironment) GetExternalName() string {
	if e.GetID() == "" {
		return resources.UndefinedExternalName
	}

	return e.GetID()
}

func (e *KymaEnvironment) GenerateK8sResourceName() string {
	if e.GetDisplayName() == "" {
		return resources.UndefinedName
	}
	resourceName, err := resources.GenerateK8sResourceName("", e.GetDisplayName())
	if err != nil {
		e.AddComment(fmt.Sprintf("cannot generate Kyma environment resource name: %s", err))
	}
	return resourceName
}
