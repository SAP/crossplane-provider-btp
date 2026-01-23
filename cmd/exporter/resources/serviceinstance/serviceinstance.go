package serviceinstance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/configparam"
	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/export"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/client"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

const (
	KIND_NAME = "serviceinstance"
)

var (
	Exporter = serviceinstanceExporter{}
	param    = configparam.StringSlice(KIND_NAME, "The ID of the service instance to export.").
		WithFlagName(KIND_NAME)
)

func init() {
	resources.RegisterKind(Exporter)
}

type serviceinstanceExporter struct{}

var _ resources.Kind = serviceinstanceExporter{}

func (e serviceinstanceExporter) Param() configparam.ConfigParam {
	return param
}

func (e serviceinstanceExporter) KindName() string {
	return KIND_NAME
}

func (e serviceinstanceExporter) Export(ctx context.Context, btpClient *client.Client, eventHandler export.EventHandler, _ bool) error {
	// TODO: ValueOrAsk for subaccounts
	selectedSubaccounts := []string{"0b8ae568-e1f5-4286-87f5-ca2844e39a29"}
	if len(selectedSubaccounts) == 0 {
		eventHandler.Warn(errors.New("no subaccounts selected to export service instances from"))
		return nil
	}

	cli := btpClient.BtpCli
	for _, subaccountID := range selectedSubaccounts {
		slog.DebugContext(ctx, "Exporting service instances for subaccount", "subaccountID", subaccountID)
		instances, err := cli.ListServiceInstances(ctx, subaccountID)
		if err != nil {
			eventHandler.Warn(fmt.Errorf("failed to retrieve service instance list: %w", err))
		}
		// TODO: ValueOrAsk for service instances
		for _, instance := range instances {
			eventHandler.Resource(convertServiceInstanceResource(&instance))
		}
	}

	return nil
}
