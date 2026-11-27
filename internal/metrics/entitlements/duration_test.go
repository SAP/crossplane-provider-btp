package entitlements

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/require"
)

const (
	durationName  = "provider_btp_api_request_duration_seconds"
	getOperation  = "get_directory_assignments"
	putOperation  = "set_service_plans"
	responseDelay = 125 * time.Millisecond
)

func durationSample(operation, suffix string) string {
	return fmt.Sprintf(`%s_%s{operation=%q,service="entitlements"}`, durationName, suffix, operation)
}

func durationBucket(operation, upperBound string) string {
	return fmt.Sprintf(`%s_bucket{operation=%q,service="entitlements",le=%q}`, durationName, operation, upperBound)
}

func readMetricSamples(t *testing.T, body, prefix string) map[string]float64 {
	t.Helper()
	samples := make(map[string]float64)
	for line := range strings.SplitSeq(body, "\n") {
		if strings.HasPrefix(line, prefix) {
			name, value, found := strings.Cut(line, " ")
			require.True(t, found, "missing sample value in %q", line)
			count, err := strconv.ParseFloat(value, 64)
			require.NoError(t, err)
			require.NotContains(t, samples, name, "duplicate sample")
			samples[name] = count
		}
	}
	return samples
}

func TestDurationMetrics_Observations(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		counter, reg := attemptTestCollector(t)
		histogram := newHistogram()
		require.NoError(t, reg.Register(histogram))
		handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
		snapshot := func() map[string]float64 {
			return readMetricSamples(t, scrapeMetrics(t, handler), durationName)
		}

		var req *http.Request
		var resp *http.Response
		var elapsed time.Duration
		var baseErr error
		calls := 0
		base := roundTripFunc(func(input *http.Request) (*http.Response, error) {
			calls++
			require.Same(t, req, input)
			time.Sleep(elapsed)
			return resp, baseErr
		})
		rt := wrap(wrapDuration(base, "", histogram), "", counter)
		body := &unreadBody{t: t}
		dispatch := func(method, path string, code int, delay time.Duration, err error) {
			t.Helper()
			req = httptest.NewRequest(method, path, nil)
			resp = nil
			if code != 0 {
				resp = &http.Response{StatusCode: code, Body: body, Header: http.Header{"Test": {"unchanged"}}}
			}
			elapsed, baseErr = delay, err
			before := calls
			got, gotErr := rt.RoundTrip(req)
			require.Equal(t, before+1, calls)
			require.True(t, gotErr == err, "returned error changed: got %v, want %v", gotErr, err)
			if resp == nil {
				require.Nil(t, got)
			} else {
				require.Same(t, resp, got)
				require.Same(t, body, got.Body)
				require.Equal(t, code, got.StatusCode)
				require.Equal(t, http.Header{"Test": {"unchanged"}}, got.Header)
			}
		}

		require.Empty(t, snapshot())
		dispatch(http.MethodGet, getPath, http.StatusOK, responseDelay, nil)
		get := snapshot()
		require.Equal(t, float64(1), get[durationSample(getOperation, "count")])
		require.Equal(t, 0.125, get[durationSample(getOperation, "sum")])
		require.Contains(t, get, durationBucket(getOperation, "0.1"))
		require.Zero(t, get[durationBucket(getOperation, "0.1")])
		require.Equal(t, float64(1), get[durationBucket(getOperation, "0.25")])
		require.NotContains(t, get, durationSample(putOperation, "count"))

		dispatch(http.MethodPut, putPath, http.StatusTooManyRequests, 250*time.Millisecond, nil)
		put := snapshot()
		require.Equal(t, float64(1), put[durationSample(putOperation, "count")])
		require.Equal(t, 0.25, put[durationSample(putOperation, "sum")])
		require.Equal(t, float64(1), put[durationBucket(putOperation, "0.25")])

		dispatch(http.MethodPut, putPath, http.StatusServiceUnavailable, 500*time.Millisecond, nil)
		retry := snapshot()
		require.Equal(t, float64(2), retry[durationSample(putOperation, "count")])
		require.Equal(t, 0.75, retry[durationSample(putOperation, "sum")])
		require.Equal(t, float64(1), retry[durationBucket(putOperation, "0.25")])
		require.Equal(t, float64(2), retry[durationBucket(putOperation, "0.5")])
		require.Equal(t, float64(1), retry[durationSample(getOperation, "count")])

		sentinel := errors.New("fake transport error")
		dispatch(http.MethodGet, getPath, http.StatusOK, responseDelay, sentinel)
		withResponse := snapshot()
		require.Equal(t, float64(2), withResponse[durationSample(getOperation, "count")])
		require.Equal(t, 0.25, withResponse[durationSample(getOperation, "sum")])

		attemptSnapshot := func() map[string]float64 {
			return readMetricSamples(t, scrapeMetrics(t, handler), attemptName)
		}
		wantAttempts := attemptSnapshot()
		dispatch(http.MethodGet, getPath, 0, responseDelay, context.DeadlineExceeded)
		wantAttempts[attemptSample(getOperation, "timeout")]++
		require.Equal(t, wantAttempts, attemptSnapshot())
		require.Equal(t, withResponse, snapshot())
		for _, err := range []error{context.Canceled, sentinel, nil} {
			dispatch(http.MethodGet, getPath, 0, responseDelay, err)
			wantAttempts[attemptSample(getOperation, "transport_error")]++
			require.Equal(t, wantAttempts, attemptSnapshot())
			require.Equal(t, withResponse, snapshot())
		}
	})

	t.Run("in flight", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			histogram := newHistogram()
			reg := prometheus.NewRegistry()
			require.NoError(t, reg.Register(histogram))
			release := make(chan struct{}, 1)
			defer close(release)
			resp := &http.Response{StatusCode: http.StatusOK, Body: &unreadBody{t: t}}
			calls := 0
			base := roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				<-release
				return resp, nil
			})
			var got *http.Response
			var err error
			go func() {
				got, err = wrapDuration(base, "", histogram).RoundTrip(httptest.NewRequest(http.MethodGet, getPath, nil))
			}()
			synctest.Wait()
			require.Equal(t, 1, calls)
			assertSamples(t, reg)
			time.Sleep(responseDelay)
			release <- struct{}{}
			synctest.Wait()
			require.Equal(t, 1, calls)
			require.Same(t, resp, got)
			require.NoError(t, err)
			samples := readMetricSamples(t, scrapeMetrics(t, promhttp.HandlerFor(reg, promhttp.HandlerOpts{})), durationName)
			require.Equal(t, float64(1), samples[durationSample(getOperation, "count")])
			require.Equal(t, 0.125, samples[durationSample(getOperation, "sum")])
		})
	})

	t.Run("nil base", func(t *testing.T) {
		rt := wrapDuration(nil, "", newHistogram()).(*durationTransport)
		require.Equal(t, http.DefaultTransport, rt.base)
	})
}

func TestDurationMetrics_Routes(t *testing.T) {
	cases := []struct {
		name, method, target, prefix, want string
	}{
		{name: "GET with query", method: http.MethodGet, target: "https://one.example" + getPath + "?planName=test", want: getOperation},
		{name: "GET other host and query", method: http.MethodGet, target: "https://two.example" + getPath + "?subaccountGUID=test", want: getOperation},
		{name: "PUT", method: http.MethodPut, target: putPath, want: putOperation},
		{name: "GET with prefix", method: http.MethodGet, target: "/tenant" + getPath, prefix: "/tenant", want: getOperation},
		{name: "PUT with escaped prefix and query", method: http.MethodPut, target: "/tenant%2Falias" + putPath + "?test=value", prefix: "/tenant%2Falias", want: putOperation},
		{name: "unrelated Entitlements", method: http.MethodGet, target: "/entitlements/v1/entitledServices"},
		{name: "trailing slash", method: http.MethodGet, target: getPath + "/"},
		{name: "extra segment", method: http.MethodGet, target: getPath + "/extra"},
		{name: "wrong prefix", method: http.MethodGet, target: "/other" + getPath, prefix: "/tenant"},
		{name: "missing prefix", method: http.MethodGet, target: getPath, prefix: "/tenant"},
		{name: "unescaped prefix", method: http.MethodPut, target: "/tenant/alias" + putPath, prefix: "/tenant%2Falias"},
		{name: "escaped delimiter", method: http.MethodGet, target: "/entitlements/v1%2Fassignments"},
		{name: "wrong GET method", method: http.MethodGet, target: putPath},
		{name: "wrong PUT method", method: http.MethodPut, target: getPath},
		{name: "token", method: http.MethodPost, target: "/oauth/token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				histogram := newHistogram()
				reg := prometheus.NewRegistry()
				require.NoError(t, reg.Register(histogram))
				req := httptest.NewRequest(tc.method, tc.target, nil)
				url, header := *req.URL, req.Header.Clone()
				resp := &http.Response{StatusCode: http.StatusOK, Body: &unreadBody{t: t}}
				calls := 0
				base := roundTripFunc(func(input *http.Request) (*http.Response, error) {
					calls++
					require.Same(t, req, input)
					require.Equal(t, tc.method, input.Method)
					require.Equal(t, url, *input.URL)
					require.Equal(t, header, input.Header)
					time.Sleep(responseDelay)
					return resp, nil
				})
				got, err := wrapDuration(base, tc.prefix, histogram).RoundTrip(req)
				require.NoError(t, err)
				require.Same(t, resp, got)
				require.Equal(t, 1, calls)
				samples := readMetricSamples(t, scrapeMetrics(t, promhttp.HandlerFor(reg, promhttp.HandlerOpts{})), durationName)
				if tc.want == "" {
					require.Empty(t, samples)
				} else {
					require.Equal(t, float64(1), samples[durationSample(tc.want, "count")])
					require.Equal(t, 0.125, samples[durationSample(tc.want, "sum")])
				}
			})
		})
	}
}

func TestDurationMetrics_Endpoint(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := prometheus.NewRegistry()
		require.NoError(t, Register(reg))
		require.NoError(t, RegisterDuration(reg))
		var duplicate prometheus.AlreadyRegisteredError
		require.ErrorAs(t, RegisterDuration(reg), &duplicate)

		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
		before := scrapeMetrics(t, mux)
		rt := Wrap(WrapDuration(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet {
				time.Sleep(responseDelay)
				return &http.Response{StatusCode: http.StatusOK}, nil
			}
			time.Sleep(250 * time.Millisecond)
			return &http.Response{StatusCode: http.StatusTooManyRequests}, nil
		}), ""), "")
		_, err := rt.RoundTrip(httptest.NewRequest(http.MethodGet, getPath, nil))
		require.NoError(t, err)
		_, err = rt.RoundTrip(httptest.NewRequest(http.MethodPut, putPath, nil))
		require.NoError(t, err)

		after := scrapeMetrics(t, mux)
		require.Contains(t, after, "# TYPE "+durationName+" histogram\n")
		require.Contains(t, after, "# TYPE "+attemptName+" counter\n")
		want := readMetricSamples(t, before, durationName)
		for operation, seconds := range map[string]float64{getOperation: 0.125, putOperation: 0.25} {
			want[durationSample(operation, "count")]++
			want[durationSample(operation, "sum")] += seconds
			for _, bound := range []string{"0.005", "0.01", "0.025", "0.05", "0.1"} {
				key := durationBucket(operation, bound)
				if _, exists := want[key]; !exists {
					want[key] = 0
				}
			}
			for _, bound := range []string{"0.25", "0.5", "1", "2.5", "5", "10", "+Inf"} {
				want[durationBucket(operation, bound)]++
			}
		}
		require.Equal(t, want, readMetricSamples(t, after, durationName))
		wantAttempts := readMetricSamples(t, before, attemptName)
		wantAttempts[attemptSample(getOperation, "2xx")]++
		wantAttempts[attemptSample(putOperation, "429")]++
		require.Equal(t, wantAttempts, readMetricSamples(t, after, attemptName))
	})
}
