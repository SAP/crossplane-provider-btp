package httpcount_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sap/crossplane-provider-btp/internal/httpcount"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/entitlements", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	mux.HandleFunc("/subaccounts", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRoundTripper_CountsSequentialRequests(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newTestServer(t)
	rt := httpcount.New(nil)
	client := rt.Client()

	for range 5 {
		resp, err := client.Get(srv.URL + "/entitlements")
		r.NoError(err)
		_ = resp.Body.Close()
	}
	for range 2 {
		resp, err := client.Get(srv.URL + "/subaccounts")
		r.NoError(err)
		_ = resp.Body.Close()
	}

	r.Equal(uint64(7), rt.Total())
	r.Equal(uint64(5), rt.CountFor("GET", "/entitlements"))
	r.Equal(uint64(2), rt.CountFor("GET", "/subaccounts"))
	r.Equal(uint64(0), rt.CountFor("POST", "/entitlements"))
}

// TestRoundTripper_ConcurrentSafe fires many requests in parallel and
// checks the total is exact. Must be run under -race.
func TestRoundTripper_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newTestServer(t)
	rt := httpcount.New(nil)
	client := rt.Client()

	const workers = 16
	const perWorker = 50

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range perWorker {
				resp, err := client.Get(srv.URL + "/entitlements")
				if err == nil {
					_ = resp.Body.Close()
				}
			}
		})
	}
	wg.Wait()

	r.Equal(uint64(workers*perWorker), rt.Total())
	r.Equal(uint64(workers*perWorker), rt.CountFor("GET", "/entitlements"))
}

// TestRoundTripper_Reset ensures Reset zeroes all counters cleanly.
func TestRoundTripper_Reset(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newTestServer(t)
	rt := httpcount.New(nil)
	client := rt.Client()

	resp, err := client.Get(srv.URL + "/entitlements")
	r.NoError(err)
	_ = resp.Body.Close()

	r.Equal(uint64(1), rt.Total())
	rt.Reset()
	r.Equal(uint64(0), rt.Total())
	r.Empty(rt.Snapshot())
}

// TestRoundTripper_CountsFailedRequests verifies non-2xx / transport
// failures are still counted — "did we hit the network?" is intent.
func TestRoundTripper_CountsFailedRequests(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	rt := httpcount.New(nil)
	client := rt.Client()

	resp, err := client.Get(srv.URL + "/boom")
	r.NoError(err) // transport didn't fail; response is 500
	_ = resp.Body.Close()

	r.Equal(uint64(1), rt.Total())
	r.Equal(uint64(1), rt.CountFor("GET", "/boom"))
}

// TestRoundTripper_Snapshot_Independence — mutating the returned map
// must not affect internal state.
func TestRoundTripper_Snapshot_Independence(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newTestServer(t)
	rt := httpcount.New(nil)
	client := rt.Client()

	resp, err := client.Get(srv.URL + "/entitlements")
	r.NoError(err)
	_ = resp.Body.Close()

	snap := rt.Snapshot()
	snap["GET /entitlements"] = 999
	snap["GET /forged"] = 42

	r.Equal(uint64(1), rt.CountFor("GET", "/entitlements"))
	r.Equal(uint64(0), rt.CountFor("GET", "/forged"))
}
