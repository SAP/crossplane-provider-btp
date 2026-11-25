package entitlements

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var duration = newHistogram()

func newHistogram() *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "provider_btp_api_request_duration_seconds",
		Help:    "BTP API request duration in seconds until the underlying HTTP transport returns response headers.",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "operation"})
}

// RegisterDuration registers the process-wide latency histogram on the provider's metrics registry.
func RegisterDuration(reg prometheus.Registerer) error {
	return reg.Register(duration)
}

// WrapDuration measures matching Entitlements requests until the base transport
// returns a response. Install it below OAuth and body-reading middleware. The
// prefix is the escaped path of the configured server URL.
func WrapDuration(base http.RoundTripper, prefix string) http.RoundTripper {
	return wrapDuration(base, prefix, duration)
}

func wrapDuration(base http.RoundTripper, prefix string, histogram *prometheus.HistogramVec) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &durationTransport{base: base, histogram: histogram, routes: newOperationRoutes(prefix)}
}

type durationTransport struct {
	base      http.RoundTripper
	histogram *prometheus.HistogramVec
	routes    operationRoutes
}

func (t *durationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	operation := t.routes.operation(req)
	if operation == "" {
		return t.base.RoundTrip(req)
	}

	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	elapsed := time.Since(start)
	if resp != nil {
		t.histogram.WithLabelValues("entitlements", operation).Observe(elapsed.Seconds())
	}
	return resp, err
}
