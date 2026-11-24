package entitlements

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/require"
)

const (
	getPath     = "/entitlements/v1/assignments"
	putPath     = "/entitlements/v1/subaccountServicePlans"
	attemptName = "provider_btp_api_request_attempts_total"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type networkError struct {
	error
	timeout bool
}

func (e networkError) Timeout() bool   { return e.timeout }
func (e networkError) Temporary() bool { return false }
func (e networkError) Unwrap() error   { return e.error }

type unreadBody struct {
	t *testing.T
}

func (b *unreadBody) Read([]byte) (int, error) {
	b.t.Error("the wrapper read a body")
	return 0, io.EOF
}

func (b *unreadBody) Close() error {
	b.t.Error("the wrapper closed a body")
	return nil
}

func attemptSample(operation, outcome string) string {
	return fmt.Sprintf(`%s{operation=%q,result=%q,service="entitlements"}`, attemptName, operation, outcome)
}

func scrapeMetrics(t *testing.T, handler http.Handler) string {
	t.Helper()
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, resp.Code)
	return resp.Body.String()
}

func assertSamples(t *testing.T, reg *prometheus.Registry, samples ...string) {
	t.Helper()
	var actual []string
	body := scrapeMetrics(t, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	for line := range strings.SplitSeq(body, "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			actual = append(actual, line)
		}
	}
	require.ElementsMatch(t, samples, actual)
}

func attemptTestCollector(t *testing.T) (*prometheus.CounterVec, *prometheus.Registry) {
	t.Helper()
	counter := newCounter()
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(counter))
	return counter, reg
}

func TestAttemptMetrics_Result(t *testing.T) {
	cases := []struct {
		name string
		code int
		err  error
		want string
	}{
		{name: "101", code: 101, want: "other_http"},
		{name: "199", code: 199, want: "other_http"},
		{name: "200", code: 200, want: "2xx"},
		{name: "299", code: 299, want: "2xx"},
		{name: "300", code: 300, want: "3xx"},
		{name: "399", code: 399, want: "3xx"},
		{name: "400", code: 400, want: "other_4xx"},
		{name: "428", code: 428, want: "other_4xx"},
		{name: "429", code: 429, want: "429"},
		{name: "430", code: 430, want: "other_4xx"},
		{name: "499", code: 499, want: "other_4xx"},
		{name: "500", code: 500, want: "5xx"},
		{name: "599", code: 599, want: "5xx"},
		{name: "600", code: 600, want: "other_http"},
		{name: "wrapped deadline", err: fmt.Errorf("base: %w", context.DeadlineExceeded), want: "timeout"},
		{name: "wrapped network timeout", err: fmt.Errorf("base: %w", networkError{error: errors.New("network"), timeout: true}), want: "timeout"},
		{name: "timeout within another network error", err: &networkError{error: &networkError{error: errors.New("network"), timeout: true}}, want: "timeout"},
		{name: "joined network timeout", err: errors.Join(errors.New("network"), &networkError{error: errors.New("timed out"), timeout: true}), want: "timeout"},
		{name: "cancellation", err: context.Canceled, want: "transport_error"},
		{name: "non-timeout network error", err: &networkError{error: errors.New("network")}, want: "transport_error"},
		{name: "nil response and error", want: "transport_error"},
		{name: "response takes precedence", code: 429, err: context.DeadlineExceeded, want: "429"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			counter, reg := attemptTestCollector(t)
			body := &unreadBody{t: t}
			req := httptest.NewRequest(http.MethodGet, getPath, body)
			var resp *http.Response
			if tc.code != 0 {
				resp = &http.Response{StatusCode: tc.code, Body: body}
			}
			var calls int
			base := roundTripFunc(func(input *http.Request) (*http.Response, error) {
				calls++
				require.Same(t, req, input)
				return resp, tc.err
			})
			assertSamples(t, reg)
			got, err := wrap(base, "", counter).RoundTrip(req)
			if resp == nil {
				require.Nil(t, got)
			} else {
				require.Same(t, resp, got)
				require.Same(t, body, got.Body)
				require.Equal(t, tc.code, got.StatusCode)
			}
			require.True(t, err == tc.err, "returned error changed: got %v, want %v", err, tc.err)
			require.Equal(t, 1, calls)
			assertSamples(t, reg, attemptSample("get_directory_assignments", tc.want)+" 1")
		})
	}
}

func TestAttemptMetrics_Routes(t *testing.T) {
	cases := []struct {
		name, method, target, prefix, want string
	}{
		{name: "GET with query", method: http.MethodGet, target: "https://one.example" + getPath + "?planName=test", want: "get_directory_assignments"},
		{name: "GET other host and query", method: http.MethodGet, target: "https://two.example" + getPath + "?subaccountGUID=test", want: "get_directory_assignments"},
		{name: "PUT", method: http.MethodPut, target: putPath, want: "set_service_plans"},
		{name: "GET with prefix", method: http.MethodGet, target: "/tenant" + getPath, prefix: "/tenant", want: "get_directory_assignments"},
		{name: "PUT with escaped prefix", method: http.MethodPut, target: "/tenant%2Falias" + putPath, prefix: "/tenant%2Falias", want: "set_service_plans"},
		{name: "unrelated Entitlements", method: http.MethodGet, target: "/entitlements/v1/entitledServices"},
		{name: "trailing slash", method: http.MethodGet, target: getPath + "/"},
		{name: "extra segment", method: http.MethodGet, target: getPath + "/extra"},
		{name: "unexpected prefix", method: http.MethodGet, target: "/other" + getPath, prefix: "/tenant"},
		{name: "missing prefix", method: http.MethodGet, target: getPath, prefix: "/tenant"},
		{name: "escaped delimiter", method: http.MethodGet, target: "/entitlements/v1%2Fassignments"},
		{name: "wrong GET method", method: http.MethodGet, target: putPath},
		{name: "wrong PUT method", method: http.MethodPut, target: getPath},
		{name: "token", method: http.MethodPost, target: "/oauth/token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			counter, reg := attemptTestCollector(t)
			req := httptest.NewRequest(tc.method, tc.target, nil)
			url, header := *req.URL, req.Header.Clone()
			resp := &http.Response{StatusCode: http.StatusOK}
			var calls int
			base := roundTripFunc(func(input *http.Request) (*http.Response, error) {
				calls++
				require.Same(t, req, input)
				require.Equal(t, tc.method, input.Method)
				require.Equal(t, url, *input.URL)
				require.Equal(t, header, input.Header)
				return resp, nil
			})
			got, err := wrap(base, tc.prefix, counter).RoundTrip(req)
			require.NoError(t, err)
			require.Same(t, resp, got)
			require.Equal(t, 1, calls)
			if tc.want == "" {
				assertSamples(t, reg)
			} else {
				assertSamples(t, reg, attemptSample(tc.want, "2xx")+" 1")
			}
		})
	}
}

func TestAttemptMetrics_CountsAfterReturn(t *testing.T) {
	counter, reg := attemptTestCollector(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	base := roundTripFunc(func(input *http.Request) (*http.Response, error) {
		if input.URL.RawQuery == "blocked" {
			close(entered)
			<-release
		}
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	rt := wrap(base, "", counter)
	var wg sync.WaitGroup
	wg.Go(func() { _, _ = rt.RoundTrip(httptest.NewRequest(http.MethodGet, getPath+"?blocked", nil)) })
	<-entered
	assertSamples(t, reg)
	unblock()
	wg.Wait()
	sample := attemptSample("get_directory_assignments", "2xx")
	assertSamples(t, reg, sample+" 1")
	const calls = 20
	for range calls {
		wg.Go(func() { _, _ = rt.RoundTrip(httptest.NewRequest(http.MethodGet, getPath, nil)) })
	}
	wg.Wait()
	assertSamples(t, reg, fmt.Sprintf("%s %d", sample, 1+calls))
}

func TestAttemptMetrics_CallerRetry(t *testing.T) {
	counter, reg := attemptTestCollector(t)
	var calls int
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: http.StatusTooManyRequests}, nil
		}
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	rt := wrap(base, "", counter)
	resp, err := rt.RoundTrip(httptest.NewRequest(http.MethodPut, putPath, nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.Equal(t, 1, calls)
	rateLimited := attemptSample("set_service_plans", "429") + " 1"
	assertSamples(t, reg, rateLimited)

	resp, err = rt.RoundTrip(httptest.NewRequest(http.MethodPut, putPath, nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, calls)
	assertSamples(t, reg, rateLimited, attemptSample("set_service_plans", "2xx")+" 1")
}

func TestAttemptMetrics_Endpoint(t *testing.T) {
	reg := prometheus.NewRegistry()
	require.NoError(t, Register(reg))
	var duplicate prometheus.AlreadyRegisteredError
	require.ErrorAs(t, Register(reg), &duplicate)
	sentinel := prometheus.NewGauge(prometheus.GaugeOpts{Name: "unrelated_metric", Help: "Sentinel for registry coexistence."})
	require.NoError(t, reg.Register(sentinel))
	sentinel.Set(7)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	before := scrapeMetrics(t, mux)
	rt := Wrap(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet {
			return &http.Response{StatusCode: http.StatusTooManyRequests}, nil
		}
		return &http.Response{StatusCode: http.StatusAccepted}, nil
	}), "")
	_, err := rt.RoundTrip(httptest.NewRequest(http.MethodGet, getPath, nil))
	require.NoError(t, err)
	_, err = rt.RoundTrip(httptest.NewRequest(http.MethodPut, putPath, nil))
	require.NoError(t, err)

	after := scrapeMetrics(t, mux)
	require.Contains(t, after, "# TYPE "+attemptName+" counter\n")
	require.Contains(t, after, "\nunrelated_metric 7\n")
	readSamples := func(body string) map[string]float64 {
		samples := make(map[string]float64)
		for line := range strings.SplitSeq(body, "\n") {
			if strings.HasPrefix(line, attemptName) {
				sample, value, found := strings.Cut(line, " ")
				require.True(t, found, "missing sample value in %q", line)
				count, err := strconv.ParseFloat(value, 64)
				require.NoError(t, err)
				samples[sample] = count
			}
		}
		return samples
	}
	expected := readSamples(before)
	expected[attemptSample("get_directory_assignments", "429")]++
	expected[attemptSample("set_service_plans", "2xx")]++
	require.Equal(t, expected, readSamples(after))
}
