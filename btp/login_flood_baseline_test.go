package btp

// Characterization: proves the failure-retry flood that PR (a) documents. The raw
// oauth2 token source (what sharedOAuthClient uses today) caches only successful
// tokens, so while a credential fails/locks it re-hits the token endpoint on every
// call — the production flood (~5.6 logins/sec into a locked account). The fix and
// its test live in the circuit-breaker PR; this only nails down the broken baseline.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// tokenEndpoint is a fake UAA /oauth/token that counts POSTs and can be flipped
// between failing (401) and succeeding.
type tokenEndpoint struct {
	hits atomic.Int64
	fail atomic.Bool
}

func (e *tokenEndpoint) start(t *testing.T) *clientcredentials.Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.hits.Add(1)
		if e.fail.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "token_type": "bearer", "expires_in": 3600,
		})
	}))
	t.Cleanup(srv.Close)
	// Pin AuthStyle so 1 endpoint hit == 1 login attempt. (Default AuthStyleAutoDetect
	// sends 2 hits per failed fetch — the separate 2x amplifier in
	// TestIAS_AutoDetectDoublesFailedAttempts, not what this measures.)
	return &clientcredentials.Config{ClientID: "c", ClientSecret: "s", TokenURL: srv.URL, AuthStyle: oauth2.AuthStyleInParams}
}

// The break: while the credential fails, the raw ReuseTokenSource re-hits the
// endpoint on every call. 50 calls -> 50 logins, unbounded.
func TestLoginFlood_RawSourceFloods(t *testing.T) {
	t.Parallel()
	ep := &tokenEndpoint{}
	ep.fail.Store(true)
	src := ep.start(t).TokenSource(context.Background())

	const N = 50
	for i := 0; i < N; i++ {
		if _, err := src.Token(); err == nil {
			t.Fatal("expected failure from always-401 endpoint")
		}
	}
	if got := ep.hits.Load(); got != N {
		t.Errorf("raw source hits = %d, want %d (unbounded flood — the bug)", got, N)
	}
}
