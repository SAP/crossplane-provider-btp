package entitlement

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/configparam"
	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/export"

	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

const KIND_NAME = "entitlement"

const (
	paramNameSubaccount = "entitlement-subaccount"
	paramNameService    = "entitlement-service"
)

var (
	Exporter        = entitlementExporter{}
	subaccountParam = configparam.String(paramNameSubaccount, "UUID of a BTP subaccount. Used in combination with '--kind "+KIND_NAME+"'").
		WithFlagName(paramNameSubaccount)
	serviceNameParam = configparam.String(paramNameService, "Technical name of a BTP service. Used in combination with '--kind "+KIND_NAME+"'").
		WithFlagName(paramNameService)
)

func init() {
	resources.RegisterKind(Exporter)
	export.AddConfigParams(subaccountParam)
	export.AddConfigParams(serviceNameParam)
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
	slog.Debug("Subaccount selected", "subaccount", saGuid)
	slog.Debug("Technical service name selected", "service", serviceName)

	req := btpClient.EntitlementsServiceClient.
		GetDirectoryAssignments(ctx).
		SubaccountGUID(saGuid).
		AssignedServiceName(serviceName)

	result, _, err := req.Execute()
	if err != nil {
		return fmt.Errorf("failed to get full list of entitlements: %w", err)
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
	for _, svc := range svcs {
		plans, hasPlans := svc.GetServicePlansOk()
		if !hasPlans || len(plans) == 0 {
			eventHandler.Warn(fmt.Errorf("no service plan found for service: %s", *svc.Name))
			continue
		}

		for _, plan := range plans {
			assignments, hasAssignments := plan.GetAssignmentInfoOk()
			if !hasAssignments || len(assignments) == 0 {
				eventHandler.Warn(fmt.Errorf("no assignment info found for service: %s plan: %s", *svc.Name, *plan.Name))
				continue
			}

			for _, a := range assignments {
				eventHandler.Resource(convertEntitlementResource(&svc, &plan, &a))
			}
		}
	}

	return nil
}
