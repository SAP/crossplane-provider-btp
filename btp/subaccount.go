package btp

import (
	"context"

	accountsserviceclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

func (c *Client) GetBTPSubaccount(
	ctx context.Context, subaccountGUID string,
) (*accountsserviceclient.SubaccountResponseObject, error) {
	btpSubaccount, _, err := c.AccountsServiceClient.SubaccountOperationsAPI.GetSubaccount(ctx, subaccountGUID).Execute()
	return btpSubaccount, err
}
