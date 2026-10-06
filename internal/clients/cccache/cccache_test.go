package cccache

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

func resetCache() {
	mu.Lock()
	cache = map[string]*http.Client{}
	mu.Unlock()
}

// tokenServer counts token POSTs (logins) and serves everything else 200.
func tokenServer(t *testing.T, expiresIn int) (url string, logins *atomic.Int64) {
	t.Helper()
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			n.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "bearer", "expires_in": expiresIn})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &n
}

func cfgFor(url string) *clientcredentials.Config {
	return &clientcredentials.Config{ClientID: "cid", ClientSecret: "sec", TokenURL: url + "/oauth/token"}
}

// Same credential reused across many uses -> one login.
func TestCCCache_ReusesToken(t *testing.T) {
	resetCache()
	url, logins := tokenServer(t, 3600)
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		c := HTTPClient(ctx, cfgFor(url))
		if _, err := c.Get(url + "/resource"); err != nil {
			t.Fatalf("get: %v", err)
		}
	}
	if got := logins.Load(); got != 1 {
		t.Errorf("logins across 50 uses = %d, want 1 (token reused)", got)
	}
}

// Concurrent uses of one credential -> one login.
func TestCCCache_ConcurrentOneLogin(t *testing.T) {
	resetCache()
	url, logins := tokenServer(t, 3600)
	ctx := context.Background()
	c := HTTPClient(ctx, cfgFor(url))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := c.Get(url + "/resource")
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	if got := logins.Load(); got != 1 {
		t.Errorf("logins across 50 concurrent uses = %d, want 1", got)
	}
}

// Same key returns the same cached client instance.
func TestCCCache_SameKeySameClient(t *testing.T) {
	resetCache()
	url, _ := tokenServer(t, 3600)
	ctx := context.Background()
	a := HTTPClient(ctx, cfgFor(url))
	b := HTTPClient(ctx, cfgFor(url))
	if a != b {
		t.Error("same credential must return the same cached *http.Client")
	}
}

// Distinct credentials get distinct clients and distinct logins.
func TestCCCache_DistinctCredsDistinctLogins(t *testing.T) {
	resetCache()
	u1, l1 := tokenServer(t, 3600)
	u2, l2 := tokenServer(t, 3600)
	ctx := context.Background()
	c1 := HTTPClient(ctx, cfgFor(u1))
	c2 := HTTPClient(ctx, cfgFor(u2))
	if c1 == c2 {
		t.Fatal("distinct credentials must not share a client")
	}
	_, _ = c1.Get(u1 + "/r")
	_, _ = c2.Get(u2 + "/r")
	if l1.Load() != 1 || l2.Load() != 1 {
		t.Errorf("distinct-credential logins = (%d,%d), want (1,1)", l1.Load(), l2.Load())
	}
}

// Configs differing only in EndpointParams or AuthStyle must not share a client.
func TestCCCache_KeyIncludesEndpointParamsAndAuthStyle(t *testing.T) {
	resetCache()
	ctx := context.Background()
	base := HTTPClient(ctx, cfgFor("http://x"))

	withParams := cfgFor("http://x")
	withParams.EndpointParams = map[string][]string{"resource": {"r"}}
	if HTTPClient(ctx, withParams) == base {
		t.Error("distinct EndpointParams must not share a client")
	}

	withStyle := cfgFor("http://x")
	withStyle.AuthStyle = oauth2.AuthStyleInParams
	if HTTPClient(ctx, withStyle) == base {
		t.Error("distinct AuthStyle must not share a client")
	}
}

// A cancelled creation ctx must not break later token refreshes.
func TestCCCache_SurvivesCancelledCtx(t *testing.T) {
	resetCache()
	// expires_in below oauth2's 10s expiryDelta -> every request refetches the token.
	url, logins := tokenServer(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	c := HTTPClient(ctx, cfgFor(url))
	cancel()
	if _, err := c.Get(url + "/resource"); err != nil {
		t.Fatalf("get after ctx cancel: %v", err)
	}
	if logins.Load() != 1 {
		t.Errorf("logins = %d, want 1", logins.Load())
	}
}
