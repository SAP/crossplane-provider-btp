package entitlement

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

const KindName = "entitlement"

const (
	paramNameAutoAssigned = "entitlement-auto-assigned"
)

var (
	fullCache     resources.ResourceCache[*entitlement]
	selectedCache resources.ResourceCache[*entitlement]
	registry      = resources.NewRegistry()

	entitlementParam = configparam.StringSlice(KindName, "Service plan name (or name fragment) to export. If specified, it must be a valid regex expression.").
		WithFlagName(KindName).
		WithExample("--entitlement '.*\\bcis\\b.*'")
	autoAssignedParam = configparam.Bool(paramNameAutoAssigned, "Include service plans that are automatically assigned to all subaccounts.\nUsed in combination with '--kind "+KindName+"'").
		WithFlagName(paramNameAutoAssigned)
)

func init() {
	resources.RegisterKind(exporter{})
	export.AddConfigParams(entitlementParam)
	export.AddConfigParams(autoAssignedParam)
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
	slog.DebugContext(ctx, "Export auto-assigned entitlements", "auto-assigned", autoAssignedParam.Value())

	cache, err := Get(ctx, btpClient)
	if err != nil {
		return fmt.Errorf("failed to get cache with entitlements: %w", err)
	}
	slog.DebugContext(ctx, "Entitlements in cache after user selection", "count", cache.Len())

	if cache.Len() == 0 {
		eventHandler.Warn(fmt.Errorf("no entitlements found"))
	} else {
		for _, e := range cache.All() {
			convert(ctx, btpClient, e, eventHandler, resolveReferences)
		}
	}

	return nil
}

func Get(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*entitlement], error) {
	if selectedCache != nil {
		return selectedCache, nil
	}

	fc, err := getFullCache(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get full cache with entitlements: %w", err)
	}
	slog.DebugContext(ctx, "Entitlements in full cache before selection", "count", fc.Len())

	// Create a shallow copy of the full cache to keep only selected entitlements,
	// so that the full cache remains unchanged for other resources that might need it during their export.
	cache := fc.Copy()

	// Let the user select entitlements to export.
	widgetValues := cache.ValuesForSelection()
	entitlementParam.WithPossibleValuesFn(func() ([]string, error) {
		return widgetValues.Values(), nil
	})

	selectedEntitlements, err := entitlementParam.ValueOrAsk(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter value: %s, %w", entitlementParam.GetName(), err)
	}
	slog.DebugContext(ctx, "Selected entitlements", "entitlements", selectedEntitlements)

	// Keep only selected entitlements in the cache.
	cache.KeepSelectedOnly(selectedEntitlements)
	selectedCache = cache

	return selectedCache, nil
}

func getFullCache(ctx context.Context, btpClient *btpcli.BtpCli) (resources.ResourceCache[*entitlement], error) {
	if fullCache != nil {
		return fullCache, nil
	}

	// Let the user select relevant subaccounts.
	saCache, err := subaccount.Get(ctx, btpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve subaccount cache: %w", err)
	}
	slog.DebugContext(ctx, "Subaccounts in cache after user selection", "count", saCache.Len())

	// Retrieve service assignments for all selected subaccounts.
	var svcs []btpcli.AssignedService
	for _, saId := range saCache.AllIDs() {
		saAssignments, err := btpClient.ListServiceAssignments(ctx, saId)
		if err != nil {
			return nil, fmt.Errorf("failed to get entitlements for subaccount %s: %w", saId, err)
		}
		svcs = append(svcs, saAssignments...)
	}
	slog.DebugContext(ctx, "Service assignments retrieved by BTP CLI", "count", len(svcs))

	// Wrap service assignments for internal processing and caching.
	entitlements := serviceToEntitlement(svcs)
	slog.DebugContext(ctx, "Total entitlements", "count", len(entitlements))

	// Create cache and store all entitlements.
	fullCache := resources.NewResourceCache[*entitlement]()
	fullCache.Store(entitlements...)

	return fullCache, nil
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

func ExportEntitlement(ctx context.Context, btpClient *btpcli.BtpCli, subaccountID string, serviceName string, planName string, eventHandler export.EventHandler, resolveReferences bool) error {
	e, found, err := getEntitlement(ctx, btpClient, subaccountID, serviceName, planName)
	if err != nil {
		return fmt.Errorf("failed to retrieve cloud management instance for subaccount %s: %w", subaccountID, err)
	}

	if found {
		convert(ctx, btpClient, e, eventHandler, resolveReferences)
	} else {
		convertDefault(ctx, btpClient, e, eventHandler, resolveReferences)
	}

	return nil
}

func convert(ctx context.Context, btpClient *btpcli.BtpCli, e *entitlement, eventHandler export.EventHandler, resolveReferences bool) {
	if register(ctx, e) {
		eventHandler.Resource(convertEntitlementResource(ctx, btpClient, e, eventHandler, resolveReferences))
	}
}

func convertDefault(ctx context.Context, btpClient *btpcli.BtpCli, e *entitlement, eventHandler export.EventHandler, resolveReferences bool) {
	if register(ctx, e) {
		eventHandler.Resource(convertDefaultEntitlementResource(ctx, btpClient, e, eventHandler, resolveReferences))
	}
}

func register(ctx context.Context, e *entitlement) bool {
	success := registry.Register(e.GetID())
	if !success {
		slog.DebugContext(ctx, "Entitlement already exported", "subaccount", e.assignment.EntityID, "instance", e.GetID())
	}
	return success
}

func getEntitlement(ctx context.Context, btpClient *btpcli.BtpCli, subaccountID string, serviceName string, planName string) (*entitlement, bool, error) {
	cache, err := getFullCache(ctx, btpClient)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get full cache with entitlements: %w", err)
	}

	for _, e := range cache.All() {
		if e.assignment.EntityID == subaccountID &&
			e.serviceName == serviceName &&
			e.planName == planName {
			return e, true, nil
		}
	}

	return defaultCloudManagement(subaccountID, serviceName, planName), false, nil
}

func defaultCloudManagement(subaccountID string, serviceName string, planName string) *entitlement {
	return &entitlement{
		serviceName: serviceName,
		planName:    planName,
		assignment: &btpcli.AssignmentInfo{
			EntityID:                subaccountID,
			ModifiedDate:            0,               // needed for ID generation
			UnlimitedAmountAssigned: true,            // enable:true
			Amount:                  amountUnlimited, // enable:true
			ParentAmount:            amountUnlimited, // enable:true
		},
		ResourceWithComment: yaml.NewResourceWithComment(nil),
	}
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
	return fmt.Sprintf("%s-%s-%g",
		e.serviceName,
		e.planName,
		e.assignment.Amount)
}

func (e *entitlement) GetExternalName() string {
	return ""
}

func (e *entitlement) GenerateK8sResourceName() string {
	if e.serviceName == "" || e.planName == "" || e.assignment.EntityID == "" {
		return resources.UndefinedName
	}

	resourceName := fmt.Sprintf("%s-%s-%s", e.serviceName, e.planName, e.assignment.EntityID)

	resourceName, err := resources.GenerateK8sResourceName("", resourceName, "")
	if err != nil {
		e.AddComment(fmt.Sprintf("cannot generate entitlement resource name: %s", err))
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
