package btp

import (
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	entitlementmetrics "github.com/sap/crossplane-provider-btp/internal/metrics/entitlements"
	entclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
)

type attemptRoundTripper func(*http.Request) (*http.Response, error)

func (f *attemptRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return (*f)(req) }

func attemptResponse(req *http.Request, code int, body string) *http.Response {
	return &http.Response{
		Request:    req,
		StatusCode: code,
		Status:     http.StatusText(code),
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type durationSample struct {
	count uint64
	sum   float64
}

func entitlementsDurationSnapshot(t *testing.T, reg *prometheus.Registry) map[string]durationSample {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	values := make(map[string]durationSample)
	for _, family := range families {
		if family.GetName() != "provider_btp_api_request_duration_seconds" {
			continue
		}
		for _, sample := range family.GetMetric() {
			labels := make(map[string]string)
			for _, label := range sample.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			require.Len(t, labels, 2)
			require.Equal(t, "entitlements", labels["service"])
			require.Contains(t, []string{"get_directory_assignments", "set_service_plans"}, labels["operation"])
			histogram := sample.GetHistogram()
			require.NotNil(t, histogram)
			values[labels["operation"]] = durationSample{count: histogram.GetSampleCount(), sum: histogram.GetSampleSum()}
		}
	}
	return values
}

func TestClient_EntitlementsAttemptMetrics(t *testing.T) {
	// Changes the base transport hook; do not run in parallel.
	const (
		server        = "https://cis.example"
		prefix        = "/region%2Ftest"
		assignments   = prefix + "/entitlements/v1/assignments"
		servicePlans  = prefix + "/entitlements/v1/subaccountServicePlans"
		globalAccount = "/accounts/v1/globalAccount"
		environments  = "/provisioning/v1/availableEnvironments"
		testBearer    = "Bearer test-access-token"
		accountJSON   = `{"commercialModel":"","consumptionBased":false,"createdDate":0,"description":"","displayName":"test","geoAccess":"","globalAccountGUID":"test","guid":"test","licenseType":"","parentGUID":"","parentType":""}`
	)
	reg := prometheus.NewRegistry()
	require.NoError(t, entitlementmetrics.Register(reg))
	require.NoError(t, entitlementmetrics.RegisterDuration(reg))
	durationCounts := func(t *testing.T) map[string]uint64 {
		t.Helper()
		counts := make(map[string]uint64)
		for operation, sample := range entitlementsDurationSnapshot(t, reg) {
			counts[operation] = sample.count
		}
		return counts
	}
	type sampleKey struct{ operation, result string }
	gather := func() map[sampleKey]float64 {
		t.Helper()
		families, err := reg.Gather()
		require.NoError(t, err)
		values := make(map[sampleKey]float64)
		for _, family := range families {
			if family.GetName() != "provider_btp_api_request_attempts_total" {
				continue
			}
			for _, sample := range family.GetMetric() {
				labels := make(map[string]string)
				for _, label := range sample.GetLabel() {
					labels[label.GetName()] = label.GetValue()
				}
				require.Len(t, labels, 3)
				require.Equal(t, "entitlements", labels["service"])
				require.NotEmpty(t, labels["operation"])
				require.NotEmpty(t, labels["result"])
				values[sampleKey{labels["operation"], labels["result"]}] = sample.GetCounter().GetValue()
			}
		}
		return values
	}

	var seen []string
	var failToken bool
	base := attemptRoundTripper(func(req *http.Request) (*http.Response, error) {
		path := req.URL.EscapedPath()
		seen = append(seen, req.Method+" "+path)
		if req.Body != nil {
			defer func() { require.NoError(t, req.Body.Close()) }()
		}
		if path == "/oauth/token" {
			require.Equal(t, http.MethodPost, req.Method)
			if failToken {
				return attemptResponse(req, http.StatusUnauthorized, `{"error":"invalid_client"}`), nil
			}
			return attemptResponse(req, http.StatusOK, `{"access_token":"test-access-token","token_type":"Bearer","expires_in":3600}`), nil
		}
		require.Equal(t, testBearer, req.Header.Get("Authorization"))
		switch path {
		case globalAccount:
			// This matching GET must stay uncounted when reached via Accounts.
			resp := attemptResponse(req, http.StatusFound, "")
			resp.Header.Set("Location", assignments)
			return resp, nil
		case environments:
			return attemptResponse(req, http.StatusOK, `{"availableEnvironments":[]}`), nil
		case assignments:
			if req.URL.Query().Get("serviceName") == "test" {
				return attemptResponse(req, http.StatusOK, `{}`), nil
			}
			return attemptResponse(req, http.StatusOK, accountJSON), nil
		case servicePlans:
			return attemptResponse(req, http.StatusAccepted, `"job-accepted"`), nil
		default:
			return nil, errors.New("unexpected test request: " + req.Method + " " + path)
		}
	})
	original := newBaseHTTPClientFn
	newBaseHTTPClientFn = func() *http.Client { return &http.Client{Transport: &base} }
	t.Cleanup(func() { newBaseHTTPClientFn = original })

	cred := &Credentials{CISCredential: &CISCredential{GrantType: grantTypeClientCredentials}}
	cis := cred.CISCredential
	cis.Uaa.Clientid, cis.Uaa.Clientsecret, cis.Uaa.Url = "test-client", "test-secret", server
	cis.Endpoints.AccountsServiceUrl = server
	cis.Endpoints.ProvisioningServiceUrl = server
	cis.Endpoints.EntitlementsServiceUrl = server + prefix
	cfg := createConfig(cred, tokenURL, authenticationParams(cred))
	cfg.AuthStyle = oauth2.AuthStyleInHeader // One request even when this test's token endpoint rejects credentials.
	client := createClient(cred, cfg)
	before := gather()
	wantDurationCounts := durationCounts(t)

	account, _, err := client.AccountsServiceClient.GlobalAccountOperationsAPI.GetGlobalAccount(t.Context()).Execute()
	require.NoError(t, err)
	require.Equal(t, "test", account.GetGuid())
	_, _, err = client.ProvisioningServiceClient.GetAvailableEnvironments(t.Context()).Authorization("unused-sdk-header").Execute()
	require.NoError(t, err)
	require.Equal(t, before, gather(), "OAuth and other clients must not count, even on a matching GET")
	require.Equal(t, wantDurationCounts, durationCounts(t), "OAuth and other clients must not record latency")

	_, _, err = client.EntitlementsServiceClient.GetDirectoryAssignments(t.Context()).ServiceName("test").PlanName("plan").Execute()
	require.NoError(t, err)
	expected := maps.Clone(before)
	expected[sampleKey{"get_directory_assignments", "2xx"}]++
	require.Equal(t, expected, gather())
	wantDurationCounts["get_directory_assignments"]++
	require.Equal(t, wantDurationCounts, durationCounts(t))
	payload := entclient.NewSubaccountServicePlansRequestPayloadCollection([]entclient.ServicePlanAssignmentRequestPayload{
		*entclient.NewServicePlanAssignmentRequestPayload([]entclient.SubaccountServicePlanRequestPayload{
			*entclient.NewSubaccountServicePlanRequestPayload("test-subaccount"),
		}, "test", "plan"),
	})
	ack, resp, err := client.EntitlementsServiceClient.SetServicePlans(t.Context()).SubaccountServicePlansRequestPayloadCollection(*payload).Execute()
	require.NoError(t, err)
	// The generated SDK returns the response body as-is for string results.
	require.Equal(t, `"job-accepted"`, ack)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	expected[sampleKey{"set_service_plans", "2xx"}]++
	require.Equal(t, expected, gather())
	wantDurationCounts["set_service_plans"]++
	require.Equal(t, wantDurationCounts, durationCounts(t))
	wantRequests := []string{"POST /oauth/token", "GET " + globalAccount, "GET " + assignments, "GET " + environments, "GET " + assignments, "PUT " + servicePlans}
	require.Equal(t, wantRequests, seen, "all three client families must reuse one OAuth token")

	_, resp, err = client.EntitlementsServiceClient.SetServicePlans(t.Context()).Execute()
	require.ErrorContains(t, err, "subaccountServicePlansRequestPayloadCollection is required")
	require.Nil(t, resp)
	require.Equal(t, wantRequests, seen, "invalid payload must not send a request")
	require.Equal(t, expected, gather())
	require.Equal(t, wantDurationCounts, durationCounts(t))

	failToken = true
	fresh := createClient(cred, cfg)
	_, resp, err = fresh.EntitlementsServiceClient.GetDirectoryAssignments(t.Context()).Execute()
	var tokenErr *oauth2.RetrieveError
	require.ErrorAs(t, err, &tokenErr)
	require.Equal(t, http.StatusUnauthorized, tokenErr.Response.StatusCode)
	require.Nil(t, resp)
	require.Equal(t, append(wantRequests, "POST /oauth/token"), seen, "OAuth failure must not dispatch an Entitlements request")
	require.Equal(t, expected, gather())
	require.Equal(t, wantDurationCounts, durationCounts(t))

	t.Run("preserves HTTP client policies", func(t *testing.T) {
		jar, err := cookiejar.New(nil)
		require.NoError(t, err)
		u, err := url.Parse(server)
		require.NoError(t, err)
		jar.SetCookies(u, []*http.Cookie{{Name: "test", Value: "value", Path: "/"}})
		base := attemptRoundTripper(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, testBearer, req.Header.Get("Authorization"))
			require.Equal(t, "test=value", req.Header.Get("Cookie"))
			_, hasDeadline := req.Context().Deadline()
			require.True(t, hasDeadline)
			resp := attemptResponse(req, http.StatusFound, "")
			resp.Header.Set("Location", "/somewhere-else")
			return resp, nil
		})
		source := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-access-token"})
		oauth := &oauth2.Transport{Base: &base, Source: source}
		stopRedirect := errors.New("test redirect policy")
		redirect := func(*http.Request, []*http.Request) error { return stopRedirect }
		shared := &http.Client{Transport: oauth, Timeout: time.Minute, Jar: jar, CheckRedirect: redirect}
		entitlements := createEntitlementsServiceClient(cred, shared)
		require.Same(t, oauth, shared.Transport)
		require.Same(t, &base, oauth.Base)
		require.Equal(t, source, oauth.Source)
		require.Same(t, jar, shared.Jar)
		require.Equal(t, time.Minute, shared.Timeout)
		require.ErrorIs(t, shared.CheckRedirect(nil, nil), stopRedirect)

		_, resp, err := entitlements.GetDirectoryAssignments(t.Context()).Execute()
		require.ErrorIs(t, err, stopRedirect)
		require.Equal(t, http.StatusFound, resp.StatusCode)
		expected[sampleKey{"get_directory_assignments", "3xx"}]++
		require.Equal(t, expected, gather())
		wantDurationCounts["get_directory_assignments"]++
		require.Equal(t, wantDurationCounts, durationCounts(t))
	})

	t.Run("typed nil debugger is not dereferenced during construction", func(t *testing.T) {
		var debugger *RoundTripDebugger
		oauth := &oauth2.Transport{Base: debugger}
		shared := &http.Client{Transport: oauth}
		require.NotNil(t, createEntitlementsServiceClient(cred, shared))
		require.Same(t, oauth, shared.Transport)
		require.Equal(t, debugger, oauth.Base)
	})
}

type slowResponseBody struct {
	io.Reader
	started bool
	closed  bool
}

func (b *slowResponseBody) Read(p []byte) (int, error) {
	if !b.started {
		b.started = true
		time.Sleep(2 * time.Second)
	}
	return b.Reader.Read(p)
}

func (b *slowResponseBody) Close() error {
	b.closed = true
	return nil
}

type debuggerLogger struct {
	logging.Logger
	onDebug func()
}

func (l debuggerLogger) Debug(_ string, _ ...any) { l.onDebug() }

func TestClient_EntitlementsDurationExcludesDebuggerBody(t *testing.T) {
	// Changes the package logger; do not run in parallel.
	synctest.Test(t, func(t *testing.T) {
		reg := prometheus.NewRegistry()
		require.NoError(t, entitlementmetrics.RegisterDuration(reg))
		before := entitlementsDurationSnapshot(t, reg)
		body := &slowResponseBody{Reader: strings.NewReader(`{"fetchErrorFromExternalProviderRegistry":true}`)}
		calls, logs := 0, 0
		original := log
		SetLogger(debuggerLogger{Logger: logging.NewNopLogger(), onDebug: func() {
			logs++
			require.True(t, body.started, "debugger must read the original response body before logging")
			require.True(t, body.closed, "debugger must finish draining the original body before logging")
		}})
		t.Cleanup(func() { SetLogger(original) })

		base := attemptRoundTripper(func(req *http.Request) (*http.Response, error) {
			calls++
			require.Equal(t, "/entitlements/v1/assignments", req.URL.EscapedPath())
			require.Equal(t, "Bearer test-access-token", req.Header.Get("Authorization"))
			time.Sleep(125 * time.Millisecond)
			resp := attemptResponse(req, http.StatusOK, "")
			resp.Body = body
			return resp, nil
		})
		debugger := &RoundTripDebugger{base: &base}
		source := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-access-token"})
		oauth := &oauth2.Transport{Source: source, Base: debugger}
		shared := &http.Client{Transport: oauth}
		cred := &Credentials{CISCredential: &CISCredential{}}
		cred.CISCredential.Endpoints.EntitlementsServiceUrl = "https://cis.example"
		client := createEntitlementsServiceClient(cred, shared)

		start := time.Now()
		assignments, _, err := client.GetDirectoryAssignments(t.Context()).Execute()
		require.NoError(t, err)
		require.True(t, assignments.GetFetchErrorFromExternalProviderRegistry(), "SDK must decode the body after the debugger drains it")
		require.Equal(t, 2125*time.Millisecond, time.Since(start))
		require.Equal(t, 1, calls)
		require.Equal(t, 1, logs)
		require.Same(t, oauth, shared.Transport)
		require.Same(t, debugger, oauth.Base)
		require.Same(t, &base, debugger.base)
		require.Equal(t, source, oauth.Source)

		after := entitlementsDurationSnapshot(t, reg)
		const get = "get_directory_assignments"
		require.Equal(t, before[get].count+1, after[get].count)
		require.InDelta(t, 0.125, after[get].sum-before[get].sum, 1e-9)
		delete(before, get)
		delete(after, get)
		require.Equal(t, before, after, "no other operation should change")
	})
}
