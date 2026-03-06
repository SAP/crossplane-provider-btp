package servicebinding

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceinstance"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

const (
	KindName = "servicebinding"
)

var (
	fullCache     resources.ResourceCache[*serviceBinding]
	selectedCache resources.ResourceCache[*serviceBinding]

	bindingParam = configparam.StringSlice(KindName, "Service binding ID or regex expression for name.").
		WithFlagName(KindName)
)

func init() {
	resources.RegisterKind(exporter{})
	export.AddConfigParams(bindingParam)
}

type exporter struct{}

var _ resources.Kind = exporter{}

func (e exporter) Param() configparam.ConfigParam {
	return nil
}

func (e exporter) KindName() string {
	return KindName
}

func (e exporter) Export(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, resolveReferences bool) error {
	cache, err := Get(ctx, btpClient)
	if err != nil {
		return fmt.Errorf("failed to get cache with service bindings: %w", err)
	}
	slog.DebugContext(ctx, "Service bindings in cache after user selection", "count", cache.Len())

	if cache.Len() == 0 {
		eventHandler.Warn(fmt.Errorf("no service bindings found"))
	} else {
		for _, e := range cache.All() {
			convert(ctx, btpClient, e, eventHandler, resolveReferences)
		}
	}

	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*serviceBinding], error) {
	if selectedCache != nil {
		return selectedCache, nil
	}

	fc, err := getFullCache(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get full cache with service bindings: %w", err)
	}
	slog.DebugContext(ctx, "Service bindings in full cache before selection", "count", fc.Len())

	// Create a shallow copy of the full cache to keep only selected bindings,
	// so that the full cache remains unchanged for other resources that might need it during their export.
	cache := fc.Copy()

	// Let the user select service bindings to export.
	widgetValues := cache.ValuesForSelection()
	bindingParam.WithPossibleValuesFn(func() ([]string, error) {
		return widgetValues.Values(), nil
	})

	selectedEntitlements, err := bindingParam.ValueOrAsk(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter value: %s, %w", bindingParam.GetName(), err)
	}
	slog.DebugContext(ctx, "Selected service bindings", "bindings", selectedEntitlements)

	// Keep only selected bindings in the cache.
	cache.KeepSelectedOnly(selectedEntitlements)
	selectedCache = cache

	return selectedCache, nil
}

func convert(ctx context.Context, btpClient *btpcli.BtpCli, sb *serviceBinding, eventHandler export.EventHandler, resolveReferences bool) {
	exportPrerequisiteResources(ctx, btpClient, sb, eventHandler, resolveReferences)
	eventHandler.Resource(convertServiceBindingResource(ctx, btpClient, sb, eventHandler, resolveReferences))
}

func exportPrerequisiteResources(ctx context.Context, btpClient *btpcli.BtpCli, sb *serviceBinding, eventHandler export.EventHandler, resolveReferences bool) {
	exportServiceInstance(ctx, btpClient, sb, eventHandler, resolveReferences)
}

func exportServiceInstance(ctx context.Context, btpClient *btpcli.BtpCli, sb *serviceBinding, eventHandler export.EventHandler, resolveReferences bool) {
	siID := sb.ServiceInstanceID
	siName, err := serviceinstance.ExportInstance(ctx, btpClient, siID, eventHandler, resolveReferences)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to export service instance", "id", siID, "error", err)
	}

	// Set Service Instance reference.
	if siName != "" {
		sb.ServiceInstanceName = siName
	}
}

func getFullCache(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*serviceBinding], error) {
	if fullCache != nil {
		return fullCache, nil
	}

	// Let the user select relevant subaccounts.
	saCache, err := subaccount.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve subaccount cache: %w", err)
	}
	slog.DebugContext(ctx, "Subaccounts in cache after user selection", "count", saCache.Len())

	// Retrieve all service bindings from selected subaccounts.
	var btpBindings []btpcli.ServiceBinding
	for _, saId := range saCache.AllIDs() {
		b, err := btpClient.ListServiceBindings(ctx, saId)
		if err != nil {
			return nil, fmt.Errorf("failed to get service bindings for subaccount %s: %w", saId, err)
		}
		btpBindings = append(btpBindings, b...)
	}
	slog.DebugContext(ctx, "Total service bindings returned by BTP CLI", "count", len(btpBindings))

	// Store service bindings in cache.
	bindings := make([]*serviceBinding, len(btpBindings))
	for i, sb := range btpBindings {
		bindings[i] = &serviceBinding{
			ServiceBinding:      &sb,
			ResourceWithComment: yaml.NewResourceWithComment(nil),
		}
	}
	fullCache = resources.NewResourceCache[*serviceBinding]()
	fullCache.Store(bindings...)

	return fullCache, nil
}

func GetServiceInstanceBindings(ctx context.Context, btpClient *btpcli.BtpCli, instanceId string) (resources.ResourceCache[*serviceBinding], error) {
	cache, err := getFullCache(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve service binding cache: %w", err)
	}

	c := resources.NewResourceCache[*serviceBinding]()
	for _, sb := range cache.All() {
		if sb.ServiceInstanceID == instanceId {
			c.Set(sb)
		}
	}

	return c, nil
}

type serviceBinding struct {
	*btpcli.ServiceBinding
	*yaml.ResourceWithComment
}

var _ resources.BtpResource = &serviceBinding{}

func (sb *serviceBinding) GetID() string {
	return sb.ID
}

func (sb *serviceBinding) GetDisplayName() string {
	return sb.Name
}

func (sb *serviceBinding) GetExternalName() string {
	return sb.ID
}

func (sb *serviceBinding) GenerateK8sResourceName() string {
	resourceName, err := resources.GenerateK8sResourceName(sb.GetID(), sb.GetDisplayName(), KindName)
	if err != nil {
		sb.AddComment(fmt.Sprintf("cannot generate service manager resource name: %s", err))
	}

	return resourceName
}
