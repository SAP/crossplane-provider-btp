package tfclient

import (
	"net/http"
	"net/http/httptrace"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Metrics are registered on the controller-runtime registry, so they appear on the
// provider's existing /metrics endpoint. Session-pool metrics are labelled by
// global_account (one series per ProviderConfig); HTTP metrics are process-global
// because all sessions share one transport.
var (
	reg = promauto.With(ctrlmetrics.Registry)

	poolSize = reg.NewGaugeVec(prometheus.GaugeOpts{
		Name: "btp_tf_session_pool_size",
		Help: "Configured size of the Terraform BTP session pool per global account.",
	}, []string{"global_account"})

	poolDirty = reg.NewGaugeVec(prometheus.GaugeOpts{
		Name: "btp_tf_session_pool_dirty",
		Help: "Session-pool slots currently flagged dirty (terminated server-side session).",
	}, []string{"global_account"})

	poolLogins = reg.NewCounterVec(prometheus.CounterOpts{
		Name: "btp_tf_session_pool_logins_total",
		Help: "Logins performed to warm a session-pool slot.",
	}, []string{"global_account"})

	poolCheckouts = reg.NewCounterVec(prometheus.CounterOpts{
		Name: "btp_tf_session_pool_checkouts_total",
		Help: "Reuses of an already-warm session-pool slot. Reuse ratio = checkouts/logins.",
	}, []string{"global_account"})

	poolEvictions = reg.NewCounterVec(prometheus.CounterOpts{
		Name: "btp_tf_session_pool_evictions_total",
		Help: "Session-pool slot evictions by cause.",
	}, []string{"global_account", "reason"})

	asyncCallbacks = reg.NewCounterVec(prometheus.CounterOpts{
		Name: "btp_tf_async_callbacks_total",
		Help: "Completed async Terraform operations by op and result.",
	}, []string{"op", "result"})

	httpInFlight = reg.NewGauge(prometheus.GaugeOpts{
		Name: "btp_tf_cli_http_in_flight",
		Help: "In-flight HTTP requests to the BTP CLI server.",
	})

	httpRequests = reg.NewCounterVec(prometheus.CounterOpts{
		Name: "btp_tf_cli_http_requests_total",
		Help: "HTTP requests to the BTP CLI server by status code and method.",
	}, []string{"code", "method"})

	httpDuration = reg.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "btp_tf_cli_http_request_duration_seconds",
		Help:    "HTTP request duration to the BTP CLI server.",
		Buckets: prometheus.DefBuckets,
	}, []string{"code", "method"})

	httpConnections = reg.NewCounterVec(prometheus.CounterOpts{
		Name: "btp_tf_cli_http_connections_total",
		Help: "HTTP connections obtained for BTP CLI requests, by keep-alive reuse.",
	}, []string{"reused"})
)

// instrumentedTransport wraps base with request-level metrics (count, duration,
// in-flight) and a connection-reuse counter so the MaxIdleConnsPerHost tuning is
// observable in prod.
func instrumentedTransport(base http.RoundTripper) http.RoundTripper {
	base = connReuseTripper{base: base}
	base = promhttp.InstrumentRoundTripperDuration(httpDuration, base)
	base = promhttp.InstrumentRoundTripperCounter(httpRequests, base)
	return promhttp.InstrumentRoundTripperInFlight(httpInFlight, base)
}

type connReuseTripper struct{ base http.RoundTripper }

func (c connReuseTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			httpConnections.WithLabelValues(strconv.FormatBool(info.Reused)).Inc()
		},
	}
	return c.base.RoundTrip(r.WithContext(httptrace.WithClientTrace(r.Context(), trace)))
}
