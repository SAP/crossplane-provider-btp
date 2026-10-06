package environments

// Proves the CF login-reuse cache: cfenvironment builds a go-cfclient per Observe
// (config.New = an eager CF UAA login). Cache the authenticated config per
// credential so repeated Observes reuse one login. Non-parallel: shares the
// package-global cfConfigCache.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cloudfoundry/go-cfclient/v3/config"
)

// fakeCFAPI serves CF service discovery (GET /) + the UAA token endpoint,
// counting logins (token POSTs).
type fakeCFAPI struct {
	logins atomic.Int64
}

func (f *fakeCFAPI) start(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			f.logins.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "at", "token_type": "bearer", "expires_in": 3600, "refresh_token": "rt",
			})
			return
		}
		// discovery: point login + uaa back at this server
		_ = json.NewEncoder(w).Encode(map[string]any{
			"links": map[string]any{
				"login":   map[string]string{"href": "http://" + r.Host},
				"uaa":     map[string]string{"href": "http://" + r.Host},
				"app_ssh": map[string]any{"meta": map[string]string{"oauth_client": "ssh"}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func resetCFCache() {
	cfCacheMu.Lock()
	cfConfigCache = map[string]*config.Config{}
	cfCacheMu.Unlock()
}

// N Observes with the same credential reuse one login.
func TestCFCache_ReusesLogin(t *testing.T) {
	resetCFCache()
	fake := &fakeCFAPI{}
	url := fake.start(t)
	for i := 0; i < 50; i++ {
		if _, err := newOrganizationClient("org", url, "guid", "u", "pw", "sap.ids"); err != nil {
			t.Fatalf("newOrganizationClient: %v", err)
		}
	}
	if got := fake.logins.Load(); got != 1 {
		t.Errorf("CF logins across 50 Observes = %d, want 1 (config cached)", got)
	}
}

// Concurrent Observes collapse to one login.
func TestCFCache_ConcurrentOneLogin(t *testing.T) {
	resetCFCache()
	fake := &fakeCFAPI{}
	url := fake.start(t)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = newOrganizationClient("org", url, "guid", "u", "pw", "sap.ids") }()
	}
	wg.Wait()
	if got := fake.logins.Load(); got != 1 {
		t.Errorf("CF logins across 50 concurrent Observes = %d, want 1", got)
	}
}

// Distinct credentials (different API URLs) get distinct cached configs.
func TestCFCache_DistinctCredsDistinctLogins(t *testing.T) {
	resetCFCache()
	f1, f2 := &fakeCFAPI{}, &fakeCFAPI{}
	u1, u2 := f1.start(t), f2.start(t)
	_, _ = newOrganizationClient("org", u1, "guid", "u", "pw", "sap.ids")
	_, _ = newOrganizationClient("org", u2, "guid", "u", "pw", "sap.ids")
	if g1, g2 := f1.logins.Load(), f2.logins.Load(); g1 != 1 || g2 != 1 {
		t.Errorf("distinct-credential logins = (%d,%d), want (1,1)", g1, g2)
	}
}

// A failed login is not cached: the next Observe retries.
func TestCFCache_FailedLoginNotCached(t *testing.T) {
	resetCFCache()
	// No server: config.New cannot discover/login -> error, must not be cached.
	if _, err := newOrganizationClient("org", "http://127.0.0.1:1", "guid", "u", "pw", ""); err == nil {
		t.Fatal("expected login error")
	}
	cfCacheMu.Lock()
	_, ok := cfConfigCache["http://127.0.0.1:1\x00u\x00pw\x00"]
	cfCacheMu.Unlock()
	if ok {
		t.Error("failed login must not be cached")
	}
}
