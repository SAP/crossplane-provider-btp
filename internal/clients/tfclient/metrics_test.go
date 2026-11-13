package tfclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The async completion callback records op + result on btp_tf_async_callbacks_total.
func TestAsyncCallbackMetric(t *testing.T) {
	ac := NewAPICallbacks(nil, func(context.Context, client.Client, types.NamespacedName, ...xpv1.Condition) error {
		return nil
	})
	name := types.NamespacedName{Name: "r", Namespace: "ns"}

	before := testutil.ToFloat64(asyncCallbacks.WithLabelValues("create", "success"))
	_ = ac.Create(name, false)(nil, context.Background())
	if got := testutil.ToFloat64(asyncCallbacks.WithLabelValues("create", "success")) - before; got != 1 {
		t.Fatalf("async success delta = %v, want 1", got)
	}

	beforeErr := testutil.ToFloat64(asyncCallbacks.WithLabelValues("update", "error"))
	_ = ac.Update(name, false)(context.DeadlineExceeded, context.Background())
	if got := testutil.ToFloat64(asyncCallbacks.WithLabelValues("update", "error")) - beforeErr; got != 1 {
		t.Fatalf("async error delta = %v, want 1", got)
	}
}

// Sequential requests over the instrumented, tuned transport reuse the keep-alive
// connection, visible on btp_tf_cli_http_connections_total{reused="true"}.
func TestHTTPConnectionReuseMetric(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	hc := &http.Client{Transport: instrumentedTransport(tunedTransport(2))}

	beforeReq := testutil.ToFloat64(httpRequests.WithLabelValues("200", "get"))
	beforeReuse := testutil.ToFloat64(httpConnections.WithLabelValues("true"))

	for i := 0; i < 2; i++ {
		resp, err := hc.Get(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	if got := testutil.ToFloat64(httpRequests.WithLabelValues("200", "get")) - beforeReq; got != 2 {
		t.Fatalf("http requests delta = %v, want 2", got)
	}
	if got := testutil.ToFloat64(httpConnections.WithLabelValues("true")) - beforeReuse; got < 1 {
		t.Fatalf("reused-connection delta = %v, want >= 1 (keep-alive reuse)", got)
	}
}
