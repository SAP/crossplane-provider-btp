package serviceplan

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

var (
	planCache resources.ResourceCache[*servicePlan]
)

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*servicePlan], error) {
	if planCache != nil {
		return planCache, nil
	}

	// Let the user select relevant subaccounts.
	saCache, err := subaccount.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve subaccount cache: %w", err)
	}
	slog.DebugContext(ctx, "Subaccounts in cache after user selection", "count", saCache.Len())

	// Retrieve all service plans from selected subaccounts.
	var btpPlans []btpcli.ServicePlan
	for _, saId := range saCache.AllIDs() {
		p, err := btpClient.ListServicePlans(ctx, saId)
		if err != nil {
			return nil, fmt.Errorf("failed to get service plans for subaccount %s: %w", saId, err)
		}
		btpPlans = append(btpPlans, p...)
	}
	slog.DebugContext(ctx, "Total service plans returned by BTP CLI", "count", len(btpPlans))

	// Store service plans in cache.
	plans := make([]*servicePlan, len(btpPlans))
	for i, sp := range btpPlans {
		plans[i] = &servicePlan{
			ServicePlan: &sp,
		}
	}
	planCache = resources.NewResourceCache[*servicePlan]()
	planCache.Store(plans...)

	return planCache, nil
}

type servicePlan struct {
	*btpcli.ServicePlan
}

var _ resources.BtpResource = &servicePlan{}

func (sp *servicePlan) GetID() string {
	return sp.ID
}

func (sp *servicePlan) GetDisplayName() string {
	return sp.Name
}

func (sp *servicePlan) GetExternalName() string {
	return sp.ID
}

func (sp *servicePlan) GenerateK8sResourceName() string {
	// Not applicable, as Service Plans are not exported/managed as separate K8s resources.
	return ""
}
