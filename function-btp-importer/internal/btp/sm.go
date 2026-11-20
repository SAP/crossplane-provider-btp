package btp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/crossplane/function-sdk-go/errors"
)

const callTimeout = 10 * time.Second

// drainBody reads and discards the response body to allow HTTP connection reuse.
func drainBody(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
}

// decodeJSON decodes the response body into v, then drains any trailing bytes
// (json.Decoder stops at the end of the first value) so the underlying
// connection can be reused. Callers keep their own error wrapping.
func decodeJSON(resp *http.Response, v any) error {
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return err
	}
	drainBody(resp)
	return nil
}

// SMClient is an authenticated client for the SAP Service Manager API.
// If the client created a temporary admin binding, Close() will delete it.
type SMClient struct {
	client  *http.Client
	url     string
	cleanup func() error
}

// SMAuthError indicates a permanent credential failure (401/403) when acquiring
// the SM admin binding. The caller should treat this as Fatal.
type SMAuthError struct {
	err error
}

func (e *SMAuthError) Error() string { return e.err.Error() }
func (e *SMAuthError) Unwrap() error { return e.err }

// httpStatusError carries the HTTP status of a failed BTP API call. Callers
// classify with errors.As; a plain error without it means no HTTP status
// exists (network, request-build, or decode failure).
type httpStatusError struct {
	status int
	err    error
}

func (e *httpStatusError) Error() string { return e.err.Error() }
func (e *httpStatusError) Unwrap() error { return e.err }

// NewSMClient acquires an SM admin binding for the given subaccount and returns
// an authenticated client. If no existing binding is found, one is created and
// will be deleted when Close() is called.
//
// Returns (nil, *SMAuthError) on permanent auth failures (caller should Fatal).
// Returns (nil, error) on transient failures (caller should Warning).
//
// TODO(#39): migrate to the v2 named binding API
// (/accounts/v2/subaccounts/{id}/serviceManagerBindings) to eliminate
// contention with provider-btp, which holds the v1 binding during every
// reconcile. The v2 API supports multiple named bindings per subaccount.
func NewSMClient(ctx context.Context, cisClient *http.Client, accountsURL, subaccountID string) (*SMClient, error) {
	binding, err := getOrCreateSMBinding(ctx, cisClient, accountsURL, subaccountID)
	if err != nil {
		var se *httpStatusError
		if errors.As(err, &se) && (se.status == http.StatusUnauthorized || se.status == http.StatusForbidden) {
			return nil, &SMAuthError{err: err}
		}
		return nil, err
	}

	httpClient := NewOAuthClient(ctx, binding.uaaURL+"/oauth/token", binding.clientID, binding.clientSecret)

	var cleanup func() error
	if binding.createdByUs {
		//nolint:contextcheck // intentionally detached from request ctx; Close() runs after RunFunction returns
		cleanup = func() error {
			smCtx, cancel := context.WithTimeout(context.Background(), callTimeout)
			defer cancel()
			return deleteSMBinding(smCtx, cisClient, accountsURL, subaccountID)
		}
	}

	return &SMClient{client: httpClient, url: binding.smURL, cleanup: cleanup}, nil
}

// Close deletes the temporary SM admin binding if one was created.
func (c *SMClient) Close() error {
	if c == nil || c.cleanup == nil {
		return nil
	}
	return c.cleanup()
}

// SMResource identifies a resource returned by an SM list lookup. Beyond the
// ID it carries the linkage fields callers use to verify that a name match is
// the resource they expect before adopting it.
type SMResource struct {
	ID                string
	ServicePlanID     string // instances: the plan the instance was created from
	ServiceInstanceID string // bindings: the instance the binding belongs to
}

// FindServiceInstance queries the SM API for a service instance by name.
// A nil result with a nil error means no match.
func (c *SMClient) FindServiceInstance(ctx context.Context, name string) (*SMResource, error) {
	endpoint, err := url.JoinPath(c.url, "/v1/service_instances")
	if err != nil {
		return nil, errors.Wrap(err, "cannot build SM service instances URL")
	}
	return findSMResource(ctx, c.client, endpoint, name)
}

// FindServiceBinding queries the SM API for a service binding by name.
// A nil result with a nil error means no match.
func (c *SMClient) FindServiceBinding(ctx context.Context, name string) (*SMResource, error) {
	endpoint, err := url.JoinPath(c.url, "/v1/service_bindings")
	if err != nil {
		return nil, errors.Wrap(err, "cannot build SM service bindings URL")
	}
	return findSMResource(ctx, c.client, endpoint, name)
}

// GetServicePlanIdentity resolves a service plan ID to the catalog names of
// its offering and of the plan itself. It returns catalog_name rather than
// name because catalog_name is the field the provider resolves composition
// spec offering/plan names against — comparing any other field could disagree
// with the provider's own resolution.
func (c *SMClient) GetServicePlanIdentity(ctx context.Context, planID string) (offeringName, planName string, err error) {
	if planID == "" {
		return "", "", errors.New("cannot resolve plan identity: empty service plan ID")
	}

	planURL, err := url.JoinPath(c.url, "/v1/service_plans/", planID)
	if err != nil {
		return "", "", errors.Wrap(err, "cannot build SM service plan detail URL")
	}
	var plan smPlanDetailResponse
	if err := c.getJSON(ctx, planURL, &plan); err != nil {
		return "", "", errors.Wrapf(err, "cannot get SM service plan %q", planID)
	}
	if plan.ServiceOfferingID == "" {
		return "", "", errors.Errorf("SM service plan %q has no service offering ID", planID)
	}

	offeringURL, err := url.JoinPath(c.url, "/v1/service_offerings/", plan.ServiceOfferingID)
	if err != nil {
		return "", "", errors.Wrap(err, "cannot build SM service offering detail URL")
	}
	var offering smOfferingDetailResponse
	if err := c.getJSON(ctx, offeringURL, &offering); err != nil {
		return "", "", errors.Wrapf(err, "cannot get SM service offering %q", plan.ServiceOfferingID)
	}

	return offering.CatalogName, plan.CatalogName, nil
}

// getJSON performs one timeout-bounded GET against an SM endpoint and decodes
// the 200 response into out. Any non-200 status is an error. Used only by the
// plan/offering detail lookups; list endpoints keep their bespoke handling.
func (c *SMClient) getJSON(ctx context.Context, reqURL string, out any) error {
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, reqURL, nil)
	if err != nil {
		return errors.Wrap(err, "cannot build request")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "request failed")
	}
	defer resp.Body.Close() //nolint:errcheck // close error is ignorable

	if resp.StatusCode != http.StatusOK {
		drainBody(resp)
		return errors.Errorf("request returned status %d", resp.StatusCode)
	}
	if err := decodeJSON(resp, out); err != nil {
		return errors.Wrap(err, "cannot decode response")
	}
	return nil
}

// GetBindingCredentials fetches the credentials from an SM service binding by
// name. It first finds the binding ID via fieldQuery, then GETs the full
// binding to extract the credentials object.
func (c *SMClient) GetBindingCredentials(ctx context.Context, bindingName string) (*CISCredentials, error) {
	binding, err := c.FindServiceBinding(ctx, bindingName)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot find binding %q", bindingName)
	}
	if binding == nil {
		return nil, errors.Errorf("binding %q not found in SM", bindingName)
	}

	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	reqURL, err := url.JoinPath(c.url, "/v1/service_bindings/", binding.ID)
	if err != nil {
		return nil, errors.Wrap(err, "cannot build SM binding detail URL")
	}
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "cannot build SM binding detail request")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "SM binding detail request failed")
	}
	defer resp.Body.Close() //nolint:errcheck // close error is ignorable

	if resp.StatusCode != http.StatusOK {
		drainBody(resp)
		return nil, errors.Errorf("SM binding detail request returned status %d", resp.StatusCode)
	}

	var detail smBindingDetailResponse
	if err := decodeJSON(resp, &detail); err != nil {
		return nil, errors.Wrap(err, "cannot decode SM binding detail response")
	}

	if detail.Credentials.UAA.ClientID == "" || detail.Credentials.UAA.ClientSecret == "" || detail.Credentials.UAA.URL == "" {
		return nil, errors.Errorf("binding %q credentials missing required UAA fields", bindingName)
	}

	return &detail.Credentials, nil
}

// ---------------------------------------------------------------------------
// internal
// ---------------------------------------------------------------------------

type smListResponse struct {
	Items []struct {
		ID                string `json:"id"`
		ServicePlanID     string `json:"service_plan_id"`     //nolint:tagliatelle // SM API field name
		ServiceInstanceID string `json:"service_instance_id"` //nolint:tagliatelle // SM API field name
	} `json:"items"`
}

type smPlanDetailResponse struct {
	CatalogName       string `json:"catalog_name"`        //nolint:tagliatelle // SM API field name
	ServiceOfferingID string `json:"service_offering_id"` //nolint:tagliatelle // SM API field name
}

type smOfferingDetailResponse struct {
	CatalogName string `json:"catalog_name"` //nolint:tagliatelle // SM API field name
}

type smBindingDetailResponse struct {
	ID          string         `json:"id"`
	Credentials CISCredentials `json:"credentials"`
}

func findSMResource(ctx context.Context, client *http.Client, endpoint, name string) (*SMResource, error) {
	// SM's escaping rules for fieldQuery values are not clearly documented, so
	// a quote is rejected rather than escaped — without this, the interpolated
	// query below is malformed and SM answers with a confusing API error.
	if strings.Contains(name, "'") {
		return nil, errors.Errorf("resource name %q contains a single quote, which is not supported in SM fieldQuery lookups", name)
	}

	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parse SM endpoint URL %q", endpoint)
	}
	params := u.Query()
	params.Set("fieldQuery", fmt.Sprintf("name eq '%s'", name))
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "cannot build SM list request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "SM list request failed")
	}
	defer resp.Body.Close() //nolint:errcheck // close error is ignorable

	if resp.StatusCode != http.StatusOK {
		drainBody(resp)
		return nil, errors.Errorf("SM list request returned status %d", resp.StatusCode)
	}

	var body smListResponse
	if err := decodeJSON(resp, &body); err != nil {
		return nil, errors.Wrap(err, "cannot decode SM list response")
	}

	// SM enforces unique names within a subaccount, so fieldQuery returns at most
	// one result. The loop guards against an empty ID field defensively.
	for _, item := range body.Items {
		if item.ID != "" {
			return &SMResource{
				ID:                item.ID,
				ServicePlanID:     item.ServicePlanID,
				ServiceInstanceID: item.ServiceInstanceID,
			}, nil
		}
	}
	return nil, nil
}

// smBinding holds the credentials returned by the SM admin binding endpoint.
type smBinding struct {
	clientID     string
	clientSecret string
	smURL        string
	uaaURL       string
	createdByUs  bool
}

type smBindingResponse struct {
	ClientID     string `json:"clientid"`
	ClientSecret string `json:"clientsecret"`
	SMURL        string `json:"sm_url"` //nolint:tagliatelle // BTP API field name
	UAAURL       string `json:"url"`    // BTP uses "url" (not "uaa_url") for the UAA base URL
}

func smBindingPath(accountsURL, subaccountID string) (string, error) {
	return url.JoinPath(accountsURL, "/accounts/v1/subaccounts/", subaccountID, "/serviceManagementBinding")
}

// getOrCreateSMBinding implements the GET-before-POST idempotent pattern for
// the subaccount-admin SM binding. Returns createdByUs=false if an existing
// binding was found (caller must NOT delete it), createdByUs=true if a new
// binding was created (caller should defer deleteSMBinding).
func getOrCreateSMBinding(ctx context.Context, client *http.Client, accountsURL, subaccountID string) (*smBinding, error) {
	bindPath, err := smBindingPath(accountsURL, subaccountID)
	if err != nil {
		return nil, errors.Wrap(err, "cannot build SM binding URL")
	}

	binding, err := getSMBinding(ctx, client, bindPath)
	if err == nil {
		binding.createdByUs = false
		return binding, nil
	}
	var se *httpStatusError
	if !errors.As(err, &se) || se.status != http.StatusNotFound {
		return nil, err
	}

	binding, err = createSMBinding(ctx, client, bindPath)
	if err != nil {
		return nil, err
	}
	binding.createdByUs = true
	return binding, nil
}

func getSMBinding(ctx context.Context, client *http.Client, bindPath string) (*smBinding, error) {
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, bindPath, nil)
	if err != nil {
		return nil, errors.Wrap(err, "cannot build SM binding GET request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "SM binding GET request failed")
	}
	defer resp.Body.Close() //nolint:errcheck // close error is ignorable

	if resp.StatusCode != http.StatusOK {
		drainBody(resp)
		return nil, &httpStatusError{status: resp.StatusCode, err: errors.Errorf("SM binding GET returned status %d", resp.StatusCode)}
	}

	var b smBindingResponse
	if err := decodeJSON(resp, &b); err != nil {
		return nil, errors.Wrap(err, "cannot decode SM binding GET response")
	}

	return &smBinding{
		clientID:     b.ClientID,
		clientSecret: b.ClientSecret,
		smURL:        b.SMURL,
		uaaURL:       b.UAAURL,
	}, nil
}

func createSMBinding(ctx context.Context, client *http.Client, bindPath string) (*smBinding, error) {
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, bindPath, nil)
	if err != nil {
		return nil, errors.Wrap(err, "cannot build SM binding POST request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "SM binding POST request failed")
	}
	defer resp.Body.Close() //nolint:errcheck // close error is ignorable

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		drainBody(resp)
		return nil, &httpStatusError{status: resp.StatusCode, err: errors.Errorf("SM binding POST returned status %d", resp.StatusCode)}
	}

	var b smBindingResponse
	if err := decodeJSON(resp, &b); err != nil {
		return nil, errors.Wrap(err, "cannot decode SM binding POST response")
	}

	return &smBinding{
		clientID:     b.ClientID,
		clientSecret: b.ClientSecret,
		smURL:        b.SMURL,
		uaaURL:       b.UAAURL,
	}, nil
}

func deleteSMBinding(ctx context.Context, client *http.Client, accountsURL, subaccountID string) error {
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	bindPath, err := smBindingPath(accountsURL, subaccountID)
	if err != nil {
		return errors.Wrap(err, "cannot build SM binding URL")
	}
	req, err := http.NewRequestWithContext(callCtx, http.MethodDelete, bindPath, nil)
	if err != nil {
		return errors.Wrap(err, "cannot build SM binding DELETE request")
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "SM binding DELETE request failed")
	}
	defer resp.Body.Close() //nolint:errcheck // close error is ignorable

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		drainBody(resp)
		return errors.Errorf("SM binding DELETE returned status %d", resp.StatusCode)
	}
	return nil
}
