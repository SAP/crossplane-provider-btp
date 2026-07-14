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
	// SubaccountGuidBySubdomain looks up an existing subaccount in the global
	// account by its subdomain (unique within a global account) for the
	// orphaned-external-name adoption heal path. found is false when no
	// subaccount matches; an error is returned when more than one matches
	// (refuse to guess) or the API call fails.
	//
	// createdAt is the BTP-reported creation time of the matched subaccount.
	// Callers use it to enforce the ownership check (recovery.IsOwnedByCR)
	// before actually adopting — only subaccounts born at/after the CR's own
	// creationTimestamp were plausibly created by us; anything older is a
	// brownfield resource that must be adopted explicitly.
	SubaccountGuidBySubdomain(ctx context.Context, subdomain string) (guid string, createdAt time.Time, found bool, err error)
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

// SubaccountGuidBySubdomain implements AccountsApiAccessor.
func (a *AccountsClient) SubaccountGuidBySubdomain(ctx context.Context, subdomain string) (string, time.Time, bool, error) {
	if subdomain == "" {
		return "", time.Time{}, false, nil
	}
	collection, _, err := a.btp.AccountsServiceClient.SubaccountOperationsAPI.
		GetSubaccounts(ctx).
		Execute()
	if err != nil {
		return "", time.Time{}, false, err
	}

	type match struct {
		guid    string
		created time.Time
	}
	var matches []match
	for _, sa := range collection.GetValue() {
		if sa.Subdomain == subdomain {
			// BTP accounts service returns createdDate as milliseconds since epoch.
			matches = append(matches, match{
				guid:    sa.Guid,
				created: time.UnixMilli(sa.GetCreatedDate()),
			})
		}
	}
	switch len(matches) {
	case 0:
		return "", time.Time{}, false, nil
	case 1:
		return matches[0].guid, matches[0].created, true, nil
	default:
		return "", time.Time{}, false, errors.Errorf(
			"refusing to adopt: %d subaccounts match subdomain %q in this global account", len(matches), subdomain)
	}
}
