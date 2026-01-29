package entitlement

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/configparam"
	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/export"
	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/parsan"
	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/yaml"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

const KIND_NAME = "entitlement"

const (
	paramNameService      = "entitlement-service"
	paramNameAutoAssigned = "entitlement-auto-assigned"
)

var (
	entitlementCache resources.ResourceCache[*entitlement]
	serviceNameParam = configparam.StringSlice(paramNameService, "Technical name of a BTP service. Used in combination with '--kind "+KIND_NAME+"'").
		WithFlagName(paramNameService)
	autoAssignedParam = configparam.Bool(paramNameAutoAssigned, "Include service plans that are automatically assigned to all subaccounts.\nUsed in combination with '--kind "+KIND_NAME+"'").
		WithFlagName(paramNameAutoAssigned)
)

func init() {
	resources.RegisterKind(exporter{})
	export.AddConfigParams(serviceNameParam)
	export.AddConfigParams(autoAssignedParam)
}

type exporter struct{}

var _ resources.Kind = exporter{}

func (e exporter) Param() configparam.ConfigParam {
	return nil
}

func (e exporter) KindName() string {
	return KIND_NAME
}

func (e exporter) Export(ctx context.Context, btpClient *btpcli.BtpCli, eventHandler export.EventHandler, resolveReferences bool) error {
	slog.DebugContext(ctx, "Export auto-assigned entitlements", "auto-assigned", autoAssignedParam.Value())

	cache, err := Get(ctx, btpClient)
	if err != nil {
		return fmt.Errorf("failed to get cache with entitlements: %w", err)
	}
	slog.DebugContext(ctx, "Entitlements retrieved", "count", cache.Len())

	if cache.Len() == 0 {
		eventHandler.Warn(fmt.Errorf("no entitlements found"))
	} else {
		for _, en := range cache.All() {
			eventHandler.Resource(convertEntitlementResource(ctx, btpClient, en, eventHandler, resolveReferences))
		}
	}

	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*entitlement], error) {
	if entitlementCache != nil {
		return entitlementCache, nil
	}

	saCache, err := subaccount.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve subaccount cache: %w", err)
	}
	slog.DebugContext(ctx, "Subaccounts retrieved", "count", saCache.Len())

	var svcs []btpcli.AssignedService
	for _, saId := range saCache.AllIDs() {
		saAssignments, err := btpClient.ListServiceAssignmentsBySubaccount(ctx, saId)
		if err != nil {
			return nil, fmt.Errorf("failed to get entitlements for subaccount %s: %w", saId, err)
		}
		svcs = append(svcs, saAssignments...)
	}

	entitlements := serviceToEntitlement(svcs)
	cache := resources.NewResourceCache[*entitlement]()
	cache.Store(entitlements...)
	widgetValues := cache.ValuesForSelection()
	serviceNameParam.WithPossibleValuesFn(func() ([]string, error) {
		return widgetValues.Values(), nil
	})

	selectedEntitlements, err := serviceNameParam.ValueOrAsk(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter value: %s, %w", serviceNameParam.GetName(), err)
	}
	slog.DebugContext(ctx, "Selected entitlements", "service plans", selectedEntitlements)

	cache.KeepSelectedOnly(selectedEntitlements)
	entitlementCache = cache

	return entitlementCache, nil
}

func serviceToEntitlement(assignments []btpcli.AssignedService) []*entitlement {
	var entitlements []*entitlement
	for _, svc := range assignments {
		for _, plan := range svc.ServicePlans {
			for _, assignInfo := range plan.AssignmentInfo {
				if !autoAssignedParam.Value() && assignInfo.AutoAssigned {
					continue
				}
				ent := &entitlement{
					serviceName:         svc.Name,
					planName:            plan.Name,
					assignment:          &assignInfo,
					ResourceWithComment: yaml.NewResourceWithComment(nil),
				}
				entitlements = append(entitlements, ent)
			}
		}
	}
	return entitlements
}

type entitlement struct {
	serviceName string
	planName    string
	assignment  *btpcli.AssignmentInfo
	*yaml.ResourceWithComment
}

var _ resources.BtpResource = &entitlement{}

func (e *entitlement) GetID() string {
	return fmt.Sprintf("%s-%s-%s-%d", e.assignment.EntityID, e.serviceName, e.planName, e.assignment.ModifiedDate)
}

func (e *entitlement) GetDisplayName() string {
	return fmt.Sprintf("service:%s, plan: %s, amount: %g",
		e.serviceName,
		e.planName,
		e.assignment.Amount)
}

func (e *entitlement) GetExternalName() string {
	return "no-external-name-support"
}

func (e *entitlement) GenerateK8sResourceName() string {
	if e.serviceName == "" || e.planName == "" || e.assignment.EntityID == "" {
		return resources.UNDEFINED_NAME
	}

	resourceName := fmt.Sprintf("%s-%s-%s", e.serviceName, e.planName, e.assignment.EntityID)

	names := parsan.ParseAndSanitize(resourceName, parsan.RFC1035LowerSubdomain)
	if len(names) == 0 {
		e.AddComment(fmt.Sprintf("error sanitizing entitlement name: %s", resourceName))
	} else {
		resourceName = names[0]
	}

	return resourceName
}

const amountUnlimited float64 = 2000000000

func (e *entitlement) isEnable() bool {

	// Rather hacky heuristics.
	// Case 1: Service plan with global unlimited quota is involved.
	if e.assignment.UnlimitedAmountAssigned &&
		e.assignment.Amount == amountUnlimited &&
		e.assignment.ParentAmount == amountUnlimited {
		return true
	}

	// Case 2: Service plan is assigned, but its remaining parent amount is not getting less.
	if e.assignment.Amount > 0 &&
		e.assignment.Amount == e.assignment.ParentAmount &&
		e.assignment.ParentRemainingAmount != nil &&
		e.assignment.ParentAmount == *e.assignment.ParentRemainingAmount {

		// This should not happen to service plans that have a numeric quota, because then:
		// parentRemainingAmount = parentAmount - amount - other subaccount's amount
		return true
	}

	return false
}
