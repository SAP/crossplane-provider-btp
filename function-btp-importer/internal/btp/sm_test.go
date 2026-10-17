package btp

import (
	"context"
	"net/http"
	"testing"

	"github.com/crossplane/function-sdk-go/errors"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/sap/crossplane-provider-btp/function-btp-importer/internal/btptest"
)

// bindingPath is the SM admin binding endpoint for the subaccount the binding
// tests use.
const bindingPath = "/accounts/v1/subaccounts/sub-abc/serviceManagementBinding"

// advertised are static service URLs for tests that decode a binding response
// and compare the URLs it carries; the mock's own address would be different
// on every run.
var advertised = btptest.Advertised{SM: "https://sm.example.com", UAA: "https://uaa.example.com"}

// withInstances / withBindings / withPlans / withOfferings replace one list of
// the fixture.
func withInstances(res ...btptest.SMResource) func(btptest.Config) btptest.Config {
	return func(c btptest.Config) btptest.Config {
		c.ServiceInstances = res
		return c
	}
}

func withPlans(plans []btptest.CatalogResource, offerings []btptest.CatalogResource) func(btptest.Config) btptest.Config {
	return func(c btptest.Config) btptest.Config {
		c.ServicePlans = plans
		c.ServiceOfferings = offerings
		return c
	}
}

// statusOf returns the HTTP status carried by err via *httpStatusError, or 0
// when the chain carries none (network, request-build, or decode failure).
func statusOf(err error) int {
	var se *httpStatusError
	if errors.As(err, &se) {
		return se.status
	}
	return 0
}

// ---------------------------------------------------------------------------
// FindServiceInstance
// ---------------------------------------------------------------------------

func TestFindServiceInstance(t *testing.T) {
	type args struct {
		setup func(btptest.Config) btptest.Config
		name  string
	}
	type want struct {
		res *SMResource
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"SingleMatchReturnsResourceIdentity": {
			reason: "SM returns exactly one instance matching the name — its ID and service plan ID are returned; the decoy listed first proves the lookup filters by the requested name rather than taking whatever SM lists",
			args: args{
				setup: func(c btptest.Config) btptest.Config {
					c.ServiceInstances = append([]btptest.SMResource{{Name: "other", ID: "other-uuid"}}, c.ServiceInstances...)
					return c
				},
				name: "my-instance",
			},
			want: want{res: &SMResource{ID: "si-uuid-1", ServicePlanID: "plan-uuid-1"}},
		},
		"MissingIdentityFieldsLeftEmpty": {
			reason: "a response without identity fields still yields the ID — the identity stays empty for the caller to treat as unverifiable",
			args:   args{setup: withInstances(btptest.SMResource{Name: "my-instance", ID: "si-uuid-1"}), name: "my-instance"},
			want:   want{res: &SMResource{ID: "si-uuid-1"}},
		},
		"NoMatchReturnsNil": {
			reason: "SM returns empty items — no match, nil resource with no error",
			args:   args{name: "missing"},
			want:   want{res: nil},
		},
		"MultipleMatchesReturnsFirst": {
			reason: "SM returns multiple items — first non-empty ID returned with its identity (SM enforces unique names per subaccount)",
			args: args{
				setup: withInstances(
					btptest.SMResource{Name: "dup", ID: "uuid-1", ServicePlanID: "plan-a"},
					btptest.SMResource{Name: "dup", ID: "uuid-2", ServicePlanID: "plan-b"},
				),
				name: "dup",
			},
			want: want{res: &SMResource{ID: "uuid-1", ServicePlanID: "plan-a"}},
		},
		"NameWithSingleQuote": {
			reason: "a name containing a single quote cannot be expressed in an SM fieldQuery — rejected with a clear error before any HTTP request is made (the mock is down, so a request would surface as a different error)",
			args:   args{setup: down, name: "it's-broken"},
			want:   want{err: errors.New(`resource name "it's-broken" contains a single quote, which is not supported in SM fieldQuery lookups`)},
		},
		"ServerError": {
			reason: "SM returns 5xx — error returned",
			args:   args{setup: withFaults(btptest.Fault{Status: http.StatusInternalServerError}), name: "x"},
			want:   want{err: cmpopts.AnyError},
		},
		"MalformedResponse": {
			reason: "SM returns 200 with invalid JSON — error returned",
			args:   args{setup: withFaults(btptest.Fault{Body: `}not valid json{`}), name: "x"},
			want:   want{err: cmpopts.AnyError},
		},
		"NetworkFailure": {
			reason: "server unavailable — error returned",
			args:   args{setup: down, name: "x"},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := btptest.Default()
			if tc.args.setup != nil {
				cfg = tc.args.setup(cfg)
			}
			srv := btptest.Serve(t, cfg)

			// Construct directly: NewSMClient acquires OAuth bindings and cannot be
			// used in unit tests that need a pre-built client pointed at a test server.
			sm := &SMClient{client: srv.Client(), url: srv.URL}
			got, err := sm.FindServiceInstance(context.Background(), tc.args.name)

			if diff := cmp.Diff(tc.want.err, err, equateErrors()); diff != "" {
				t.Errorf("%s\nFindServiceInstance(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.res, got); diff != "" {
				t.Errorf("%s\nFindServiceInstance(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FindServiceBinding
// ---------------------------------------------------------------------------

func TestFindServiceBinding(t *testing.T) {
	type args struct {
		setup func(btptest.Config) btptest.Config
		name  string
	}
	type want struct {
		res *SMResource
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"SingleMatchReturnsResourceIdentity": {
			reason: "SM returns exactly one binding matching the name — its ID and service instance ID are returned; the decoy listed first proves the lookup filters by the requested name",
			args: args{
				setup: func(c btptest.Config) btptest.Config {
					c.ServiceBindings = append([]btptest.SMResource{{Name: "other", ID: "other-uuid"}}, c.ServiceBindings...)
					return c
				},
				name: "my-binding",
			},
			want: want{res: &SMResource{ID: "sb-uuid-1", ServiceInstanceID: "si-uuid-1"}},
		},
		"NameWithSingleQuote": {
			reason: "a name containing a single quote cannot be expressed in an SM fieldQuery — rejected with a clear error before any HTTP request is made (the mock is down, so a request would surface as a different error)",
			args:   args{setup: down, name: "it's-broken"},
			want:   want{err: errors.New(`resource name "it's-broken" contains a single quote, which is not supported in SM fieldQuery lookups`)},
		},
		"NoMatchReturnsNil": {
			reason: "SM returns empty items — no match, nil resource with no error",
			args:   args{name: "missing"},
			want:   want{res: nil},
		},
		"ServerError": {
			reason: "SM returns 5xx — error returned",
			args:   args{setup: withFaults(btptest.Fault{Status: http.StatusInternalServerError}), name: "x"},
			want:   want{err: cmpopts.AnyError},
		},
		"MalformedResponse": {
			reason: "SM returns 200 with invalid JSON — error returned",
			args:   args{setup: withFaults(btptest.Fault{Body: `}not valid json{`}), name: "x"},
			want:   want{err: cmpopts.AnyError},
		},
		"NetworkFailure": {
			reason: "server unavailable — error returned",
			args:   args{setup: down, name: "x"},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := btptest.Default()
			if tc.args.setup != nil {
				cfg = tc.args.setup(cfg)
			}
			srv := btptest.Serve(t, cfg)

			sm := &SMClient{client: srv.Client(), url: srv.URL}
			got, err := sm.FindServiceBinding(context.Background(), tc.args.name)

			if diff := cmp.Diff(tc.want.err, err, equateErrors()); diff != "" {
				t.Errorf("%s\nFindServiceBinding(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.res, got); diff != "" {
				t.Errorf("%s\nFindServiceBinding(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetServicePlanIdentity
// ---------------------------------------------------------------------------

func TestGetServicePlanIdentity(t *testing.T) {
	type args struct {
		setup  func(btptest.Config) btptest.Config
		planID string
	}
	type identity struct {
		offering string
		plan     string
	}
	type want struct {
		identity identity
		err      error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"CatalogNamesResolved": {
			reason: "plan and offering detail responses carry both name and catalog_name — the catalog names are returned, because that is the field the provider resolves composition spec names against",
			args: args{
				setup: withPlans(
					[]btptest.CatalogResource{{ID: "plan-uuid-1", Name: "hana-display", CatalogName: "hana", ServiceOfferingID: "off-uuid-1"}},
					[]btptest.CatalogResource{{ID: "off-uuid-1", Name: "hana-cloud-display", CatalogName: "hana-cloud"}},
				),
				planID: "plan-uuid-1",
			},
			want: want{identity: identity{offering: "hana-cloud", plan: "hana"}},
		},
		"PlanNotFound": {
			reason: "404 on the plan detail GET is a verification failure — error returned",
			args:   args{planID: "gone"},
			want:   want{err: cmpopts.AnyError},
		},
		"OfferingNotFound": {
			reason: "404 on the offering detail GET is a verification failure — error returned",
			args: args{
				setup:  withPlans([]btptest.CatalogResource{{ID: "plan-uuid-1", Name: "hana", ServiceOfferingID: "off-gone"}}, nil),
				planID: "plan-uuid-1",
			},
			want: want{err: cmpopts.AnyError},
		},
		"PlanMissingOfferingID": {
			reason: "a plan response without a service offering ID cannot be resolved to an offering — error returned without an offering request (no offering is served, so a request would surface as a different error)",
			args: args{
				setup:  withPlans([]btptest.CatalogResource{{ID: "plan-uuid-1", Name: "hana"}}, nil),
				planID: "plan-uuid-1",
			},
			want: want{err: errors.New(`SM service plan "plan-uuid-1" has no service offering ID`)},
		},
		"MalformedPlanResponse": {
			reason: "SM returns 200 with invalid JSON — error returned",
			args:   args{setup: withFaults(btptest.Fault{Body: `}not valid json{`}), planID: "x"},
			want:   want{err: cmpopts.AnyError},
		},
		"ServerError": {
			reason: "SM returns 5xx — error returned",
			args:   args{setup: withFaults(btptest.Fault{Status: http.StatusInternalServerError}), planID: "x"},
			want:   want{err: cmpopts.AnyError},
		},
		"NetworkFailure": {
			reason: "server unavailable — error returned",
			args:   args{setup: down, planID: "x"},
			want:   want{err: cmpopts.AnyError},
		},
		"EmptyPlanID": {
			reason: "an empty plan ID is rejected before any request is made (the mock is down, so a request would surface as a different error)",
			args:   args{setup: down, planID: ""},
			want:   want{err: errors.New("cannot resolve plan identity: empty service plan ID")},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := btptest.Default()
			if tc.args.setup != nil {
				cfg = tc.args.setup(cfg)
			}
			srv := btptest.Serve(t, cfg)

			sm := &SMClient{client: srv.Client(), url: srv.URL}
			offering, plan, err := sm.GetServicePlanIdentity(context.Background(), tc.args.planID)
			got := identity{offering: offering, plan: plan}

			if diff := cmp.Diff(tc.want.err, err, equateErrors()); diff != "" {
				t.Errorf("%s\nGetServicePlanIdentity(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.identity, got, cmp.AllowUnexported(identity{})); diff != "" {
				t.Errorf("%s\nGetServicePlanIdentity(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// getSMBinding
// ---------------------------------------------------------------------------

func TestGetSMBinding(t *testing.T) {
	type args struct {
		setup func(btptest.Config) btptest.Config
	}
	// result is everything observable from one call: the decoded binding and
	// the HTTP status carried in the error chain (0 = none).
	type result struct {
		binding *smBinding
		status  int
	}
	type want struct {
		result result
		err    error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ExistingBinding": {
			reason: "GET 200 with valid response — binding credentials returned",
			want: want{result: result{
				binding: &smBinding{clientID: "mock-sm-cid", clientSecret: "mock-sm-csecret", smURL: "https://sm.example.com", uaaURL: "https://uaa.example.com"},
			}},
		},
		"NotFound": {
			reason: "GET 404 — error carries status 404 via *httpStatusError so the caller can branch to POST",
			args:   args{setup: withFaults(btptest.Fault{Method: http.MethodGet, Path: bindingPath, Status: http.StatusNotFound})},
			want:   want{result: result{status: http.StatusNotFound}, err: cmpopts.AnyError},
		},
		"MalformedResponse": {
			reason: "GET 200 with invalid JSON — error returned",
			args:   args{setup: withFaults(btptest.Fault{Method: http.MethodGet, Path: bindingPath, Body: `}not valid json{`})},
			want:   want{err: cmpopts.AnyError},
		},
		"NetworkFailure": {
			reason: "server unavailable — error returned",
			args:   args{setup: down},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := btptest.Default()
			cfg.Advertise = advertised
			if tc.args.setup != nil {
				cfg = tc.args.setup(cfg)
			}
			srv := btptest.Serve(t, cfg)

			binding, err := getSMBinding(context.Background(), srv.Client(), srv.URL+bindingPath)
			got := result{binding: binding, status: statusOf(err)}

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\ngetSMBinding(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.result, got, cmp.AllowUnexported(result{}, smBinding{})); diff != "" {
				t.Errorf("%s\ngetSMBinding(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// createSMBinding
// ---------------------------------------------------------------------------

func TestCreateSMBinding(t *testing.T) {
	type args struct {
		setup func(btptest.Config) btptest.Config
	}
	type result struct {
		binding *smBinding
		status  int
	}
	type want struct {
		result result
		err    error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Created": {
			reason: "POST 201 with valid response — binding credentials returned",
			want: want{result: result{
				binding: &smBinding{clientID: "mock-sm-cid", clientSecret: "mock-sm-csecret", smURL: "https://sm.example.com", uaaURL: "https://uaa.example.com"},
			}},
		},
		"Conflict": {
			reason: "POST 409 — contention, error carries status 409 via *httpStatusError",
			args:   args{setup: withFaults(btptest.Fault{Method: http.MethodPost, Path: bindingPath, Status: http.StatusConflict})},
			want:   want{result: result{status: http.StatusConflict}, err: cmpopts.AnyError},
		},
		"MalformedResponse": {
			reason: "POST 201 with invalid JSON — error returned",
			args:   args{setup: withFaults(btptest.Fault{Method: http.MethodPost, Path: bindingPath, Status: http.StatusCreated, Body: `}not valid json{`})},
			want:   want{err: cmpopts.AnyError},
		},
		"NetworkFailure": {
			reason: "server unavailable — error returned",
			args:   args{setup: down},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := btptest.Default()
			cfg.Advertise = advertised
			if tc.args.setup != nil {
				cfg = tc.args.setup(cfg)
			}
			srv := btptest.Serve(t, cfg)

			binding, err := createSMBinding(context.Background(), srv.Client(), srv.URL+bindingPath)
			got := result{binding: binding, status: statusOf(err)}

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\ncreateSMBinding(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.result, got, cmp.AllowUnexported(result{}, smBinding{})); diff != "" {
				t.Errorf("%s\ncreateSMBinding(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// deleteSMBinding
// ---------------------------------------------------------------------------

func TestDeleteSMBinding(t *testing.T) {
	type args struct {
		setup func(btptest.Config) btptest.Config
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"DeleteSucceeds200": {
			reason: "DELETE 200 — no error",
			args:   args{setup: withFaults(btptest.Fault{Method: http.MethodDelete, Path: bindingPath, Status: http.StatusOK})},
			want:   want{},
		},
		"DeleteSucceeds204": {
			reason: "DELETE 204 — no error",
			want:   want{},
		},
		"ServerError": {
			reason: "DELETE 5xx — error returned",
			args:   args{setup: withFaults(btptest.Fault{Method: http.MethodDelete, Path: bindingPath, Status: http.StatusInternalServerError})},
			want:   want{err: cmpopts.AnyError},
		},
		"NetworkFailure": {
			reason: "server unavailable — error returned",
			args:   args{setup: down},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := btptest.Default()
			if tc.args.setup != nil {
				cfg = tc.args.setup(cfg)
			}
			srv := btptest.Serve(t, cfg)

			err := deleteSMBinding(context.Background(), srv.Client(), srv.URL, "sub-abc")

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\ndeleteSMBinding(): -want error, +got error:\n%s", tc.reason, diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// getOrCreateSMBinding
// ---------------------------------------------------------------------------

func TestGetOrCreateSMBinding(t *testing.T) {
	type args struct {
		setup func(btptest.Config) btptest.Config
	}
	type result struct {
		binding *smBinding
		status  int
	}
	type want struct {
		result result
		err    error
	}

	// The mock's default binding endpoint reports an existing binding on GET;
	// rows that need the create path make GET a 404 so the code POSTs.
	getNotFound := btptest.Fault{Method: http.MethodGet, Path: bindingPath, Status: http.StatusNotFound}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"GETReturns200ReuseExisting": {
			reason: "GET 200 — existing binding reused, createdByUs=false",
			want: want{result: result{
				binding: &smBinding{clientID: "mock-sm-cid", clientSecret: "mock-sm-csecret", smURL: "https://sm.example.com", uaaURL: "https://uaa.example.com", createdByUs: false},
			}},
		},
		"GETReturns404ThenPOSTCreates": {
			reason: "GET 404 → POST creates new binding, createdByUs=true",
			args:   args{setup: withFaults(getNotFound)},
			want: want{result: result{
				binding: &smBinding{clientID: "mock-sm-cid", clientSecret: "mock-sm-csecret", smURL: "https://sm.example.com", uaaURL: "https://uaa.example.com", createdByUs: true},
			}},
		},
		"GETReturns401ShortCircuits": {
			reason: "GET 401 — short-circuits without POST, returns error with status 401 (a POST would succeed against the mock and turn the row green with a binding)",
			args:   args{setup: withFaults(btptest.Fault{Method: http.MethodGet, Path: bindingPath, Status: http.StatusUnauthorized})},
			want:   want{result: result{status: http.StatusUnauthorized}, err: cmpopts.AnyError},
		},
		"GETReturns500ShortCircuits": {
			reason: "GET 500 — short-circuits without POST, returns error with status 500 (a POST would succeed against the mock and turn the row green with a binding)",
			args:   args{setup: withFaults(btptest.Fault{Method: http.MethodGet, Path: bindingPath, Status: http.StatusInternalServerError})},
			want:   want{result: result{status: http.StatusInternalServerError}, err: cmpopts.AnyError},
		},
		"POST409ReturnsContention": {
			reason: "POST 409 — contention error returned with status 409 (caller emits Warning)",
			args:   args{setup: withFaults(getNotFound, btptest.Fault{Method: http.MethodPost, Path: bindingPath, Status: http.StatusConflict})},
			want:   want{result: result{status: http.StatusConflict}, err: cmpopts.AnyError},
		},
		"POST401ReturnsFatal": {
			reason: "POST 401 — auth error returned with status 401 (caller emits Fatal)",
			args:   args{setup: withFaults(getNotFound, btptest.Fault{Method: http.MethodPost, Path: bindingPath, Status: http.StatusUnauthorized})},
			want:   want{result: result{status: http.StatusUnauthorized}, err: cmpopts.AnyError},
		},
		"POST500ReturnsError": {
			reason: "POST 500 — error returned with status 500 (caller emits Warning)",
			args:   args{setup: withFaults(getNotFound, btptest.Fault{Method: http.MethodPost, Path: bindingPath, Status: http.StatusInternalServerError})},
			want:   want{result: result{status: http.StatusInternalServerError}, err: cmpopts.AnyError},
		},
		"NetworkFailureReturnsPlainError": {
			reason: "connection failure — plain error, no *httpStatusError in the chain (no HTTP status exists)",
			args:   args{setup: down},
			want:   want{result: result{status: 0}, err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := btptest.Default()
			cfg.Advertise = advertised
			if tc.args.setup != nil {
				cfg = tc.args.setup(cfg)
			}
			srv := btptest.Serve(t, cfg)

			binding, err := getOrCreateSMBinding(context.Background(), srv.Client(), srv.URL, "sub-abc")
			got := result{binding: binding, status: statusOf(err)}

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\ngetOrCreateSMBinding(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.result, got, cmp.AllowUnexported(result{}, smBinding{})); diff != "" {
				t.Errorf("%s\ngetOrCreateSMBinding(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetBindingCredentials
// ---------------------------------------------------------------------------

func TestGetBindingCredentials(t *testing.T) {
	type args struct {
		setup       func(btptest.Config) btptest.Config
		bindingName string
	}
	type want struct {
		creds *CISCredentials
		err   error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"BindingFoundCredentialsValid": {
			reason: "binding found with valid UAA credentials — *CISCredentials returned",
			args: args{
				setup: func(c btptest.Config) btptest.Config {
					c.Advertise = btptest.Advertised{UAA: "https://uaa.cm.example.com", Accounts: "https://accounts.example.com", Provisioning: "https://prov.example.com"}
					return c
				},
				bindingName: "my-binding",
			},
			want: want{
				creds: &CISCredentials{
					UAA: struct {
						ClientID     string `json:"clientid"`
						ClientSecret string `json:"clientsecret"`
						URL          string `json:"url"`
						SubaccountID string `json:"subaccountid"`
					}{ClientID: "mock-cm-cid", ClientSecret: "mock-cm-csecret", URL: "https://uaa.cm.example.com", SubaccountID: "7b3f9a2e-4d1c-4f8a-b5e6-2c9d8f0a1b3e"},
					Endpoints: struct {
						AccountsServiceURL     string `json:"accounts_service_url"`     //nolint:tagliatelle // BTP API uses snake_case
						ProvisioningServiceURL string `json:"provisioning_service_url"` //nolint:tagliatelle // BTP API uses snake_case
					}{AccountsServiceURL: "https://accounts.example.com", ProvisioningServiceURL: "https://prov.example.com"},
				},
			},
		},
		"BindingNotFound": {
			reason: "FindServiceBinding returns no match — error returned",
			args:   args{bindingName: "missing-binding"},
			want:   want{err: cmpopts.AnyError},
		},
		"DetailEndpointError": {
			reason: "binding ID found but detail GET returns 500 — error returned",
			args:   args{setup: withFaults(btptest.Fault{Path: "/v1/service_bindings/", Status: http.StatusInternalServerError}), bindingName: "my-binding"},
			want:   want{err: cmpopts.AnyError},
		},
		"MalformedCredentials": {
			reason: "binding found but credentials missing required UAA fields — error returned",
			args: args{
				setup:       withFaults(btptest.Fault{Path: "/v1/service_bindings/", Body: `{"id":"sb-uuid-1","credentials":{"uaa":{"clientid":"","clientsecret":"secret","url":""},"endpoints":{}}}`}),
				bindingName: "my-binding",
			},
			want: want{err: cmpopts.AnyError},
		},
		"ListEndpointError": {
			reason: "list endpoint returns 5xx — error returned before detail call is attempted",
			args:   args{setup: withFaults(btptest.Fault{Path: "/v1/service_bindings", Status: http.StatusInternalServerError}), bindingName: "my-binding"},
			want:   want{err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := btptest.Default()
			if tc.args.setup != nil {
				cfg = tc.args.setup(cfg)
			}
			srv := btptest.Serve(t, cfg)

			sm := &SMClient{client: srv.Client(), url: srv.URL}
			got, err := sm.GetBindingCredentials(context.Background(), tc.args.bindingName)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nGetBindingCredentials(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.creds, got); diff != "" {
				t.Errorf("%s\nGetBindingCredentials(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// NewSMClient
// ---------------------------------------------------------------------------

func TestNewSMClient(t *testing.T) {
	type args struct {
		setup        func(btptest.Config) btptest.Config
		subaccountID string
	}
	// result is everything observable from constructing and closing a client:
	// whether the error is the typed auth error, whether a client came back,
	// and what Close reported.
	type result struct {
		authErr   bool
		hasClient bool
		closeErr  error
	}
	type want struct {
		result result
		err    error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ExistingBindingReturnsClient": {
			reason: "GET returns existing binding — client constructed successfully and closes cleanly",
			args:   args{subaccountID: "sub-1"},
			want:   want{result: result{hasClient: true}},
		},
		"AuthErrorReturnsTypedError": {
			reason: "POST 401 — *SMAuthError returned so caller can distinguish permanent from transient failures",
			args: args{
				setup: withFaults(
					btptest.Fault{Method: http.MethodGet, Path: "/accounts/v1/subaccounts/sub-1/serviceManagementBinding", Status: http.StatusNotFound},
					btptest.Fault{Method: http.MethodPost, Path: "/accounts/v1/subaccounts/sub-1/serviceManagementBinding", Status: http.StatusUnauthorized},
				),
				subaccountID: "sub-1",
			},
			want: want{result: result{authErr: true}, err: cmpopts.AnyError},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := btptest.Default()
			if tc.args.setup != nil {
				cfg = tc.args.setup(cfg)
			}
			srv := btptest.Serve(t, cfg)

			sm, err := NewSMClient(context.Background(), srv.Client(), srv.URL, tc.args.subaccountID)
			var authErr *SMAuthError
			got := result{authErr: errors.As(err, &authErr), hasClient: sm != nil}
			if sm != nil {
				got.closeErr = sm.Close()
			}

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nNewSMClient(): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.result, got, cmp.AllowUnexported(result{}), cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nNewSMClient(): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// TestNewSMClientCloseDeletesBinding stays a focused test on a hand-rolled
// handler: it asserts that Close issues the DELETE for a binding this client
// created — a request the data fixture serves but cannot report.
func TestNewSMClientCloseDeletesBinding(t *testing.T) {
	const subID = "sub-close-test"
	bindingResp := map[string]any{
		"clientid": "sm-cid", "clientsecret": "sm-cs",
		"sm_url": "https://sm.example.com", "url": "https://uaa.example.com",
	}

	deleteCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("/accounts/v1/subaccounts/"+subID+"/serviceManagementBinding", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			http.NotFound(w, r)
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			writeJSON(t, w, bindingResp)
		case http.MethodDelete:
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"t","token_type":"Bearer","expires_in":3600}`))
	})

	srv := newTestServer(t, mux)

	sm, err := NewSMClient(context.Background(), srv.Client(), srv.URL, subID)
	if err != nil {
		t.Fatalf("NewSMClient(): unexpected error: %v", err)
	}
	if sm == nil {
		t.Fatal("NewSMClient(): expected non-nil client")
	}
	if err := sm.Close(); err != nil {
		t.Errorf("Close(): unexpected error: %v", err)
	}
	if !deleteCalled {
		t.Error("Close(): expected DELETE to binding endpoint, none received")
	}
}
