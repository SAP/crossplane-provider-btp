package servicebinding

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/yaml"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

const (
	KindName = "servicebinding"
)

var (
	bindingCache resources.ResourceCache[*serviceBinding]
)

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*serviceBinding], error) {
	if bindingCache != nil {
		return bindingCache, nil
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
	bindingCache = resources.NewResourceCache[*serviceBinding]()
	bindingCache.Store(bindings...)

	return bindingCache, nil
}

func GetBindingIdByServiceInstanceId(ctx context.Context, btpClient *btpcli.BtpCli, instanceId string) (string, bool, error) {
	cache, err := Get(ctx, btpClient)
	if err != nil {
		return "", false, fmt.Errorf("failed to retrieve service binding cache: %w", err)
	}

	for _, sb := range cache.All() {
		if sb.ServiceInstanceID == instanceId {
			return sb.ID, true, nil
		}
	}

	return "", false, nil
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
