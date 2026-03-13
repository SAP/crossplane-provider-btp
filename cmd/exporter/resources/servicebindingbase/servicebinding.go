package servicebindingbase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/yaml"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

var fullCache resources.ResourceCache[*ServiceBinding]

// ServiceBinding wraps the BTP CLI service binding with export metadata.
type ServiceBinding struct {
	*btpcli.ServiceBinding
	*yaml.ResourceWithComment
	// ServiceInstanceName is the K8s resource name of the parent service instance,
	// resolved during export and used as a reference in the generated manifest.
	ServiceInstanceName string
}

var _ resources.BtpResource = &ServiceBinding{}

func (sb *ServiceBinding) GetID() string {
	return sb.ID
}

func (sb *ServiceBinding) GetDisplayName() string {
	return sb.Name
}

func (sb *ServiceBinding) GetExternalName() string {
	return sb.ID
}

func (sb *ServiceBinding) GenerateK8sResourceName() string {
	sbName := sb.GetDisplayName()
	siID := sb.ServiceInstanceID
	if sbName == "" || siID == "" {
		return resources.UndefinedName
	}

	resourceName := fmt.Sprintf("%s-%s", sbName, siID)
	resourceName, err := resources.GenerateK8sResourceName("", resourceName, "")
	if err != nil {
		sb.AddComment(fmt.Sprintf("cannot generate resource name: %s", err))
	}

	return resourceName
}

// Get returns the full cache of all service bindings across selected subaccounts,
// fetching from BTP CLI on the first call and returning the cached result on subsequent calls.
func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*ServiceBinding], error) {
	if fullCache != nil {
		return fullCache, nil
	}

	saCache, err := subaccount.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve subaccount cache: %w", err)
	}
	slog.DebugContext(ctx, "Subaccounts in cache after user selection", "count", saCache.Len())

	var btpBindings []btpcli.ServiceBinding
	for _, saId := range saCache.AllIDs() {
		b, err := btpClient.ListServiceBindings(ctx, saId)
		if err != nil {
			return nil, fmt.Errorf("failed to get service bindings for subaccount %s: %w", saId, err)
		}
		btpBindings = append(btpBindings, b...)
	}
	slog.DebugContext(ctx, "Total service bindings returned by BTP CLI", "count", len(btpBindings))

	bindings := make([]*ServiceBinding, len(btpBindings))
	for i, sb := range btpBindings {
		sb := sb
		bindings[i] = &ServiceBinding{
			ServiceBinding:      &sb,
			ResourceWithComment: yaml.NewResourceWithComment(nil),
		}
	}
	fullCache = resources.NewResourceCache[*ServiceBinding]()
	fullCache.Store(bindings...)

	return fullCache, nil
}

// GetServiceInstanceBindings returns all service bindings for a given service instance ID.
func GetServiceInstanceBindings(ctx context.Context, btpClient *btpcli.BtpCli, instanceID string) (resources.ResourceCache[*ServiceBinding], error) {
	cache, err := Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve service binding cache: %w", err)
	}

	c := resources.NewResourceCache[*ServiceBinding]()
	for _, sb := range cache.All() {
		if sb.ServiceInstanceID == instanceID {
			c.Set(sb)
		}
	}

	return c, nil
}
