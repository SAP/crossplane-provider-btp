package environments

// Model-based concurrency test for the Cloud Foundry auth path (path 2 of 3 in
// docs/development/concurrency-and-lockout-analysis.md).
//
// cfenvironment builds a *fresh* go-cfclient per Observe via newOrganizationClient
// (config.New with UserPassword + Origin, then cfv3.New) — uncached. The password
// login fires from CreateOAuth2TokenSource → PasswordCredentialsToken, an eager
// POST to UAA /oauth/token, pinned AuthStyleInHeader (so NO autodetect doubling,
// unlike the cisclient path). This test drives that real go-cfclient config +
// token source against a fake CF API (service discovery + UAA token endpoint with
// a 5-strike lockout model), so it exercises the actual request shape; only the
// server is a model. It captures the auth boundary (login count + lockout), not
// the downstream CF resource calls.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	cfconfig "github.com/cloudfoundry/go-cfclient/v3/config"
)

// fakeCF models the CF API root (discovery) + UAA token endpoint with an IAS-style
// per-user failed-login lockout. Serialized under mu so the lock transition is
// deterministic under concurrent callers.
type fakeCF struct {
	password      string
	lockThreshold int

	mu            sync.Mutex
	failedLogins  int
	maxConsecFail int
	locked        bool

	tokenPOSTs atomic.Int64 // = login attempts against UAA
	ok         atomic.Int64
	bad        atomic.Int64
	lockedResp atomic.Int64
}

func (f *fakeCF) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/oauth/token":
			f.tokenPOSTs.Add(1)
			_ = r.ParseForm()
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.locked {
				f.lockedResp.Add(1)
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized", "error_description": "user locked"})
				return
			}
			if r.Form.Get("password") != f.password {
				f.failedLogins++
				if f.failedLogins > f.maxConsecFail {
					f.maxConsecFail = f.failedLogins
				}
				f.bad.Add(1)
				if f.failedLogins >= f.lockThreshold {
					f.locked = true
				}
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": "bad credentials"})
				return
			}
			f.failedLogins = 0
			f.ok.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "cf-at", "token_type": "bearer", "expires_in": 3600, "refresh_token": "cf-rt",
			})
		default:
			// CF API root: service discovery. Point login+uaa back at this server.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"links": map[string]any{
					"login":   map[string]string{"href": "http://" + r.Host},
					"uaa":     map[string]string{"href": "http://" + r.Host},
					"app_ssh": map[string]any{"meta": map[string]string{"oauth_client": "ssh-proxy"}},
				},
			})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// cfLoginOnce mirrors newOrganizationClient's config construction. config.New
// authenticates eagerly (a password-grant POST to UAA /oauth/token), so this is
// the login that every fresh per-Observe organizationClient performs.
func cfLoginOnce(ctx context.Context, apiURL, password string) error {
	_, err := cfconfig.New(apiURL, cfconfig.UserPassword("cf-user", password), cfconfig.Origin("sap.ids"))
	return err
}

// 100 concurrent CORRECT CF logins: uncached, so N logins hit UAA (per-Observe
// cost), and the user is never locked.
func TestCF_100ConcurrentCorrectLogins_NeverLock(t *testing.T) {
	t.Parallel()
	const N = 100
	fake := &fakeCF{password: "correct-pw", lockThreshold: 5}
	srv := fake.start(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	var failed atomic.Int64
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := cfLoginOnce(ctx, srv.URL, "correct-pw"); err != nil {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := failed.Load(); got != 0 {
		t.Errorf("failed logins = %d, want 0 (correct password)", got)
	}
	if got := fake.ok.Load(); got != N {
		t.Errorf("UAA logins = %d, want %d (uncached, one per Observe)", got, N)
	}
	if got := fake.tokenPOSTs.Load(); got != N {
		t.Errorf("token POSTs = %d, want %d (InHeader pinned, no autodetect doubling)", got, N)
	}
	fake.mu.Lock()
	locked := fake.locked
	fake.mu.Unlock()
	if locked {
		t.Error("100 concurrent CORRECT CF logins must never lock the user")
	}
}

// Wrong CF password under concurrency locks at the threshold.
func TestCF_WrongPassword_LocksAfterThreshold(t *testing.T) {
	t.Parallel()
	const N = 32
	fake := &fakeCF{password: "correct-pw", lockThreshold: 5}
	srv := fake.start(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = cfLoginOnce(ctx, srv.URL, "WRONG") }()
	}
	wg.Wait()

	fake.mu.Lock()
	locked, maxConsec := fake.locked, fake.maxConsecFail
	fake.mu.Unlock()
	if !locked {
		t.Error("wrong CF password under concurrency must lock the user")
	}
	if maxConsec < fake.lockThreshold {
		t.Errorf("max consecutive failures = %d, want ≥ threshold %d", maxConsec, fake.lockThreshold)
	}
	if got := fake.bad.Load(); got != int64(fake.lockThreshold) {
		t.Errorf("bad-credential attempts = %d, want exactly %d (InHeader pinned = 1 POST/login)", got, fake.lockThreshold)
	}
}
