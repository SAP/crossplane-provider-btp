package subaccount

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/btp"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

// AccountsApiAccessor abstraction to handle API operations by coordinating to generated api client
type AccountsApiAccessor interface {
	MoveSubaccount(ctx context.Context, subaccountGuid string, targetId string) error
	UpdateSubaccount(ctx context.Context, subaccountGuid string, payload accountclient.UpdateSubaccountRequestPayload) error
	// FindSubaccount looks up the subaccount with the given subdomain, which
	// is unique in a global account.
	FindSubaccount(ctx context.Context, subdomain string) (SubaccountMatch, bool, error)
}

// SubaccountMatch identifies the subaccount a lookup found. CreatedAt feeds
// the ownership check in internal/recovery, Region the identity check of
// adoption.
type SubaccountMatch struct {
	GUID      string
	Region    string
	CreatedAt time.Time
}

type AccountsClient struct {
	btp btp.Client
}

func (a *AccountsClient) UpdateSubaccount(ctx context.Context, subaccountGuid string, payload accountclient.UpdateSubaccountRequestPayload) error {
	_, _, err := a.btp.AccountsServiceClient.SubaccountOperationsAPI.
		UpdateSubaccount(ctx, subaccountGuid).
		UpdateSubaccountRequestPayload(payload).
		Execute()
	return err
}

func (a *AccountsClient) MoveSubaccount(ctx context.Context, subaccountGuid string, targetId string) error {
	if targetId == "" {
		return errors.New("targetId must be set for move subaccount api call")
	}
	_, _, err := a.btp.AccountsServiceClient.SubaccountOperationsAPI.
		MoveSubaccount(ctx, subaccountGuid).
		MoveSubaccountRequestPayload(
			accountclient.MoveSubaccountRequestPayload{TargetAccountGUID: targetId}).
		Execute()
	return err
}

var _ AccountsApiAccessor = &AccountsClient{}

// FindSubaccount implements AccountsApiAccessor.
func (a *AccountsClient) FindSubaccount(ctx context.Context, subdomain string) (SubaccountMatch, bool, error) {
	if subdomain == "" {
		return SubaccountMatch{}, false, nil
	}
	collection, _, err := a.btp.AccountsServiceClient.SubaccountOperationsAPI.
		GetSubaccounts(ctx).
		Execute()
	if err != nil {
		return SubaccountMatch{}, false, err
	}

	var matches []SubaccountMatch
	for _, sa := range collection.GetValue() {
		if sa.Subdomain == subdomain {
			// BTP accounts service returns createdDate as milliseconds since epoch.
			matches = append(matches, SubaccountMatch{
				GUID:      sa.Guid,
				Region:    sa.Region,
				CreatedAt: time.UnixMilli(sa.GetCreatedDate()),
			})
		}
	}
	switch len(matches) {
	case 0:
		return SubaccountMatch{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return SubaccountMatch{}, false, errors.Errorf(
			"%d subaccounts match subdomain %q in this global account", len(matches), subdomain)
	}
}
