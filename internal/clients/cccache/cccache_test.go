package cccache

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/oauth2/clientcredentials"
)

func resetCache() {
	mu.Lock()
	cache = map[string]*http.Client{}
	mu.Unlock()
}

// tokenServer counts token POSTs (logins) and serves everything else 200.
func tokenServer(t *testing.T) (url string, logins *atomic.Int64) {
	t.Helper()
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			n.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "bearer", "expires_in": 3600})
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
	url, logins := tokenServer(t)
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
	url, logins := tokenServer(t)
	ctx := context.Background()
	c := HTTPClient(ctx, cfgFor(url))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := c.Get(url + "/resource")
			if err == nil {
				resp.Body.Close()
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
	url, _ := tokenServer(t)
	ctx := context.Background()
	if HTTPClient(ctx, cfgFor(url)) != HTTPClient(ctx, cfgFor(url)) {
		t.Error("same credential must return the same cached *http.Client")
	}
}

// Distinct credentials get distinct clients and distinct logins.
func TestCCCache_DistinctCredsDistinctLogins(t *testing.T) {
	resetCache()
	u1, l1 := tokenServer(t)
	u2, l2 := tokenServer(t)
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
