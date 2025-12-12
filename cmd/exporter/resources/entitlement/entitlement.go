package entitlement

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/configparam"
	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/export"
	openapi "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

const KIND_NAME = "entitlement"

const (
	paramNameSubaccount   = "entitlement-subaccount"
	paramNameService      = "entitlement-service"
	paramNameAutoAssigned = "entitlement-auto-assigned"
)

var (
	Exporter        = entitlementExporter{}
	subaccountParam = configparam.String(paramNameSubaccount, "UUID of a BTP subaccount. Used in combination with '--kind "+KIND_NAME+"'").
		WithFlagName(paramNameSubaccount)
	serviceNameParam = configparam.String(paramNameService, "Technical name of a BTP service. Used in combination with '--kind "+KIND_NAME+"'").
		WithFlagName(paramNameService)
	autoAssignedParam = configparam.Bool(paramNameAutoAssigned, "Include service plans that are automatically available in all subaccounts.\nUsed in combination with '--kind "+KIND_NAME+"'").
		WithFlagName(paramNameAutoAssigned)
)

func init() {
	resources.RegisterKind(Exporter)
	export.AddConfigParams(subaccountParam)
	export.AddConfigParams(serviceNameParam)
	export.AddConfigParams(autoAssignedParam)
}

type entitlementExporter struct{}

var _ resources.Kind = entitlementExporter{}

func (e entitlementExporter) Param() configparam.ConfigParam {
	return nil
}

func (e entitlementExporter) KindName() string {
	return KIND_NAME
}

func (e entitlementExporter) Export(ctx context.Context, btpClient *btp.Client, eventHandler export.EventHandler, _ bool) error {
	saGuid := subaccountParam.Value()
	serviceName := serviceNameParam.Value()
	exportAutoAssigned := autoAssignedParam.Value()
	slog.Debug("Subaccount selected", "subaccount", saGuid)
	slog.Debug("Technical service name selected", "service", serviceName)
	slog.Debug("Export auto-assigned entitlements", "auto-assigned", exportAutoAssigned)

	req := btpClient.EntitlementsServiceClient.
		GetDirectoryAssignments(ctx).
		SubaccountGUID(saGuid).
		AssignedServiceName(serviceName)

	result, _, err := req.Execute()
	if err != nil {
		return fmt.Errorf("failed to get entitlements from BTP backend: %w", err)
	}

	svcs, hasSvcs := result.GetAssignedServicesOk()
	if !hasSvcs || len(svcs) == 0 {
		eventHandler.Warn(fmt.Errorf("no assigned services found"))
		return nil
	}

	// To collect the required information, three nested loops are required:
	// - assigned services
	// - assigned plans of those services
	// - assignment information of each plan
	processServices(ctx, eventHandler, svcs, exportAutoAssigned)

	return nil
}

func processServices(ctx context.Context,
	eventHandler export.EventHandler,
	svcs []openapi.AssignedServiceResponseObject,
	exportAutoAssigned bool) {

	for _, svc := range svcs {
		plans, hasPlans := svc.GetServicePlansOk()
		if !hasPlans || len(plans) == 0 {
			eventHandler.Warn(fmt.Errorf("no service plan found for service: %s", *svc.Name))
			continue
		}

		processServicePlans(ctx, eventHandler, &svc, plans, exportAutoAssigned)
	}
}

func processServicePlans(ctx context.Context,
	eventHandler export.EventHandler,
	svc *openapi.AssignedServiceResponseObject,
	plans []openapi.AssignedServicePlanResponseObject,
	exportAutoAssigned bool) {

	for _, plan := range plans {
		assignments, hasAssignments := plan.GetAssignmentInfoOk()
		if !hasAssignments || len(assignments) == 0 {
			eventHandler.Warn(fmt.Errorf("no assignment info found for service: %s plan: %s", *svc.Name, *plan.Name))
			continue
		}

		processPlanAssignments(ctx, eventHandler, svc, &plan, assignments, exportAutoAssigned)
	}
}

func processPlanAssignments(ctx context.Context,
	eventHandler export.EventHandler,
	svc *openapi.AssignedServiceResponseObject,
	plan *openapi.AssignedServicePlanResponseObject,
	assignments []openapi.AssignedServicePlanSubaccountDTO,
	exportAutoAssigned bool) {

	for _, a := range assignments {
		autoAssigned, hasAutoAssigned := a.GetAutoAssignedOk()
		if hasAutoAssigned && *autoAssigned && !exportAutoAssigned {
			if svc.Name != nil && plan.Name != nil {
				slog.Debug("Skipping auto-assigned entitlement", "service", *svc.Name, "plan", *plan.Name)
			} else {
				slog.Debug("Skipping auto-assigned entitlement for unnamed service or plan")
			}
			continue
		}

		eventHandler.Resource(convertEntitlementResource(svc, plan, &a))
	}
}
