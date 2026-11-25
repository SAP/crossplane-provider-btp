// Package entitlements records HTTP request attempts and response latency for the BTP Entitlements API.
package entitlements

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricName           = "provider_btp_api_request_attempts_total"
	metricHelp           = "BTP API request attempts counted when the underlying HTTP transport returns."
	assignmentsPath      = "/entitlements/v1/assignments"
	servicePlansPath     = "/entitlements/v1/subaccountServicePlans"
	directoryOperation   = "get_directory_assignments"
	servicePlanOperation = "set_service_plans"
)

var attempts = newCounter()

func newCounter() *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{Name: metricName, Help: metricHelp}, []string{"service", "operation", "result"})
}

// Register registers the process-wide collector on the provider's metrics registry.
func Register(reg prometheus.Registerer) error {
	return reg.Register(attempts)
}

// Wrap instruments the base transport of the Entitlements OAuth client, after
// token acquisition. The prefix is the escaped path of the configured server URL.
func Wrap(base http.RoundTripper, prefix string) http.RoundTripper {
	return wrap(base, prefix, attempts)
}

func wrap(base http.RoundTripper, prefix string, counter *prometheus.CounterVec) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &attemptTransport{base: base, counter: counter, routes: newOperationRoutes(prefix)}
}

type attemptTransport struct {
	base    http.RoundTripper
	counter *prometheus.CounterVec
	routes  operationRoutes
}

func (t *attemptTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	operation := t.routes.operation(req)
	resp, err := t.base.RoundTrip(req)
	if operation != "" {
		t.counter.WithLabelValues("entitlements", operation, result(resp, err)).Inc()
	}
	return resp, err
}

type operationRoutes struct {
	assignments  string
	servicePlans string
}

func newOperationRoutes(prefix string) operationRoutes {
	return operationRoutes{assignments: prefix + assignmentsPath, servicePlans: prefix + servicePlansPath}
}

func (r operationRoutes) operation(req *http.Request) string {
	path := req.URL.EscapedPath()
	switch {
	case req.Method == http.MethodGet && path == r.assignments:
		return directoryOperation
	case req.Method == http.MethodPut && path == r.servicePlans:
		return servicePlanOperation
	default:
		return ""
	}
}

func result(resp *http.Response, err error) string {
	if resp != nil {
		code := resp.StatusCode
		switch {
		case code == http.StatusTooManyRequests:
			return "429"
		case code >= 200 && code < 300:
			return "2xx"
		case code >= 300 && code < 400:
			return "3xx"
		case code >= 400 && code < 500:
			return "other_4xx"
		case code >= 500 && code < 600:
			return "5xx"
		default:
			return "other_http"
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || hasNetworkTimeout(err) {
		return "timeout"
	}
	return "transport_error"
}

// Check the whole chain: a non-timeout net.Error may wrap a timeout.
func hasNetworkTimeout(err error) bool {
	for err != nil {
		if networkErr, ok := err.(net.Error); ok && networkErr.Timeout() {
			return true
		}
		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			for _, inner := range joined.Unwrap() {
				if hasNetworkTimeout(inner) {
					return true
				}
			}
			return false
		}
		err = errors.Unwrap(err)
	}
	return false
}
