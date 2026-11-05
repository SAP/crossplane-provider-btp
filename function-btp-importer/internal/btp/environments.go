package btp

import (
	"context"
	"net/http"
	"net/url"

	"github.com/crossplane/function-sdk-go/errors"
)

const (
	envTypeKyma         = "kyma"
	envTypeCloudFoundry = "cloudfoundry"
)

type environmentEntry struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	EnvironmentType string `json:"environmentType"`
	PlanName        string `json:"planName"`
}

// ProvisioningClient is an authenticated client for the BTP Provisioning Service.
type ProvisioningClient struct {
	client *http.Client
	url    string
}

// NewProvisioningClient returns a ProvisioningClient using the given OAuth client and base URL.
func NewProvisioningClient(client *http.Client, provURL string) *ProvisioningClient {
	return &ProvisioningClient{client: client, url: provURL}
}

// FindKymaEnvironment returns the id of the Kyma environment matching name and
// planName. BTP does not enforce unique environment names, and a subaccount
// entitled to more than one Kyma plan (e.g. free and paid) can hold same-named
// environments on different plans; matching on planName as well is free (the
// list is already in memory) and disambiguates that case. CloudFoundry
// environments have no equivalent plan field on the CRD, so they use
// FindCloudFoundryEnvironment.
//
// Returns "" for no match, or an error if multiple environments still match
// after plan filtering (truly ambiguous — operator must resolve manually).
//
// TODO: listEnvironments is called once per environment resource in the desired
// map. Compositions with multiple environment resources will make redundant API
// calls. Consider caching the list on resolveCtx or fetching once and passing
// entries to both Find methods.
func (c *ProvisioningClient) FindKymaEnvironment(ctx context.Context, name, planName string) (string, error) {
	entries, err := c.listEnvironments(ctx)
	if err != nil {
		return "", err
	}

	var matches []environmentEntry
	for _, e := range entries {
		if e.EnvironmentType == envTypeKyma && e.Name == name && e.PlanName == planName {
			matches = append(matches, e)
		}
	}

	if len(matches) > 1 {
		return "", errors.Errorf("multiple kyma environments found for name %q plan %q", name, planName)
	}
	if len(matches) == 1 {
		return matches[0].ID, nil
	}
	return "", nil
}

// FindCloudFoundryEnvironment returns the id of the CF environment matching
// name. Unlike Kyma, CF environments have no plan field on the provider-btp
// CRD, so only name is used for matching. Returns "" for no match, or an error
// if multiple environments match (ambiguous — operator must resolve manually).
func (c *ProvisioningClient) FindCloudFoundryEnvironment(ctx context.Context, name string) (string, error) {
	entries, err := c.listEnvironments(ctx)
	if err != nil {
		return "", err
	}

	var matches []environmentEntry
	for _, e := range entries {
		if e.EnvironmentType == envTypeCloudFoundry && e.Name == name {
			matches = append(matches, e)
		}
	}

	if len(matches) > 1 {
		return "", errors.Errorf("multiple cloudfoundry environments found for name %q", name)
	}
	if len(matches) == 1 {
		return matches[0].ID, nil
	}
	return "", nil
}

func (c *ProvisioningClient) listEnvironments(ctx context.Context) ([]environmentEntry, error) {
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	reqURL, err := url.JoinPath(c.url, "/provisioning/v1/environments")
	if err != nil {
		return nil, errors.Wrap(err, "cannot build environments URL")
	}
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "cannot build environments request")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "environments request failed")
	}
	defer resp.Body.Close() //nolint:errcheck // close error is ignorable

	if resp.StatusCode != http.StatusOK {
		drainBody(resp)
		return nil, errors.Errorf("environments request returned status %d", resp.StatusCode)
	}

	var wrapper struct {
		EnvironmentInstances []environmentEntry `json:"environmentInstances"`
	}
	if err := decodeJSON(resp, &wrapper); err != nil {
		return nil, errors.Wrap(err, "cannot decode environments response")
	}
	return wrapper.EnvironmentInstances, nil
}
