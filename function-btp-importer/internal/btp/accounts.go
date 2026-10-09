package btp

import (
	"context"
	"net/http"
	"net/url"

	"github.com/crossplane/function-sdk-go/errors"
)

type subaccountListResponse struct {
	Value []struct {
		GUID      string `json:"guid"`
		Subdomain string `json:"subdomain"`
		Region    string `json:"region"`
	} `json:"value"`
}

// AccountsClient is an authenticated client for the BTP Accounts Service.
type AccountsClient struct {
	client *http.Client
	url    string
}

// NewAccountsClient returns an AccountsClient using the given OAuth client and base URL.
func NewAccountsClient(client *http.Client, accountsURL string) *AccountsClient {
	return &AccountsClient{client: client, url: accountsURL}
}

// FindSubaccount queries /accounts/v1/subaccounts and filters client-side by
// subdomain+region. Returns the guid, or "" for no match.
//
// The Accounts Service returns a single non-paginated blob — no token, no
// nextPage. BTP guarantees subdomain+region uniqueness within a global account,
// so at most one match is expected; the first match is returned.
func (c *AccountsClient) FindSubaccount(ctx context.Context, subdomain, region string) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	reqURL, err := url.JoinPath(c.url, "/accounts/v1/subaccounts")
	if err != nil {
		return "", errors.Wrap(err, "cannot build subaccounts URL")
	}
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", errors.Wrap(err, "cannot build subaccounts request")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "subaccounts request failed")
	}
	defer resp.Body.Close() //nolint:errcheck // close error is ignorable

	if resp.StatusCode != http.StatusOK {
		drainBody(resp)
		return "", errors.Errorf("subaccounts request returned status %d", resp.StatusCode)
	}

	var body subaccountListResponse
	if err := decodeJSON(resp, &body); err != nil {
		return "", errors.Wrap(err, "cannot decode subaccounts response")
	}

	for _, sa := range body.Value {
		if sa.Subdomain == subdomain && sa.Region == region {
			return sa.GUID, nil
		}
	}
	return "", nil
}
