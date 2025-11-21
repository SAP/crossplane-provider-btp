package subaccount

import (
	"context"
	"fmt"

	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/configparam"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/export"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/erratt"
)

var (
	subaccountParam = configparam.StringSlice("subaccount", "UUID of a BTP subaccount").
		WithFlagName("subaccount")
	Exporter = subaccountExporter{}
)

func init() {
	resources.RegisterKind(Exporter)
}

type subaccountExporter struct{}

var _ resources.Kind = subaccountExporter{}

func (o subaccountExporter) Param() configparam.ConfigParam {
	return subaccountParam
}

func (o subaccountExporter) Export(ctx context.Context, btpClient *btp.Client, eventHandler export.EventHandler, _ bool) error {
	accountIDs := subaccountParam.Value()

	// If no subaccount IDs are provided via command line, export all subaccounts.
	if len(accountIDs) == 0 {
		response, _, err := btpClient.AccountsServiceClient.SubaccountOperationsAPI.GetSubaccounts(ctx).Execute()
		if err != nil {
			return fmt.Errorf("failed to get full list of subaccounts: %w", err)
		}

		subaccounts := response.Value
		if len(subaccounts) == 0 {
			eventHandler.Warn(fmt.Errorf("no subaccounts found"))
		}
		for _, a := range subaccounts {
			eventHandler.Resource(convertSubaccountResource(&a))
		}
		return nil
	}

	// Export subaccounts requested from command line.
	for _, id := range accountIDs {
		exportSubaccount(ctx, btpClient, eventHandler, id)
	}

	return nil
}

// exportSubaccount exports a single subaccount by its ID.
func exportSubaccount(ctx context.Context, btpClient *btp.Client, eventHandler export.EventHandler, subaccountID string) {
	response, err := btpClient.GetBTPSubaccount(ctx, subaccountID)
	if err != nil {
		eventHandler.Warn(erratt.Errorf("failed to get subaccount: %w", err).With("uuid", subaccountID))
		return
	}

	eventHandler.Resource(convertSubaccountResource(response))
}
