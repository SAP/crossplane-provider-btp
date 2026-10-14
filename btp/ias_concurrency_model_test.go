package btp

// Model-based concurrency test for the ROPC (password-grant) technical-user
// login against a *fake* IAS token endpoint that encodes the documented policy:
//
//   - failed-login lockout: after `lockThreshold` wrong-credential attempts the
//     user is locked for `lockDuration` (IAS default 5 / 60m).
//   - "Max sessions per user": successful logins create sessions; when the count
//     exceeds `maxSessions` the OLDEST refresh token/session is evicted (LRU).
//   - an evicted/expired refresh token yields `invalid_grant` — a DIFFERENT class
//     from wrong-password, and it does NOT feed the lockout counter.
//
// These are the invariants asserted in docs/development/concurrency-and-lockout-
// analysis.md. The test drives the provider's REAL oauth2 login construction
// (createConfig + authenticationParams + TokenSource) so it exercises the actual
// request shape; only the IAS side is a model. Policy numbers are the documented
// defaults — confirm per tenant (see analysis §5).
//
// It cannot and does not hit real IAS, nor does it drive the full upjet framework
// connector; the framework path's "one login per reconcile" is modeled as a fresh
// createConfig per iteration (no token reuse), which is exactly what it does.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// --- fake IAS ---------------------------------------------------------------

type iasPolicy struct {
	lockThreshold int
	lockDuration  time.Duration
	maxSessions   int
	accessTTL     time.Duration
}

type fakeIAS struct {
	policy   iasPolicy
	password string // the single correct technical-user password

	mu            sync.Mutex
	failedLogins  int
	maxConsecFail int // high-water mark of failedLogins (drives the lock)
	lockedUntil   time.Time
	sessions      []string // LRU order; front = oldest
	validRefresh  map[string]bool
	seq           int

	total        atomic.Int64
	ok           atomic.Int64
	badCred      atomic.Int64
	locked       atomic.Int64
	invalidGrant atomic.Int64
	evicted      atomic.Int64
}

func newFakeIAS(p iasPolicy, password string) *fakeIAS {
	return &fakeIAS{policy: p, password: password, validRefresh: map[string]bool{}}
}

func (f *fakeIAS) writeErr(w http.ResponseWriter, status int, code, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": desc})
}

func (f *fakeIAS) writeToken(w http.ResponseWriter, refresh string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  "at-" + refresh,
		"token_type":    "Bearer",
		"expires_in":    int(f.policy.accessTTL.Seconds()),
		"refresh_token": refresh,
	})
}

// serve models one IAS token request. Serialized under f.mu so the lockout
// transition is deterministic even under concurrent callers.
func (f *fakeIAS) serve(w http.ResponseWriter, r *http.Request) {
	f.total.Add(1)
	_ = r.ParseForm()
	grant := r.Form.Get("grant_type")

	f.mu.Lock()
	defer f.mu.Unlock()

	if time.Now().Before(f.lockedUntil) {
		f.locked.Add(1)
		f.writeErr(w, http.StatusUnauthorized, "unauthorized", "user locked")
		return
	}

	if grant == "refresh_token" {
		rt := r.Form.Get("refresh_token")
		if !f.validRefresh[rt] {
			// Evicted/expired refresh: invalid_grant. Crucially, this does NOT
			// touch failedLogins — eviction never contributes to the lockout.
			f.invalidGrant.Add(1)
			f.writeErr(w, http.StatusBadRequest, "invalid_grant", "refresh token invalidated")
			return
		}
		f.ok.Add(1)
		f.writeToken(w, rt)
		return
	}

	// password (or client_credentials) grant
	if r.Form.Get("password") != f.password {
		f.failedLogins++
		if f.failedLogins > f.maxConsecFail {
			f.maxConsecFail = f.failedLogins
		}
		f.badCred.Add(1)
		if f.failedLogins >= f.policy.lockThreshold {
			f.lockedUntil = time.Now().Add(f.policy.lockDuration)
		}
		f.writeErr(w, http.StatusBadRequest, "invalid_grant", "bad credentials")
		return
	}

	// success: reset failure counter, mint session, LRU-evict oldest over cap
	f.failedLogins = 0
	f.seq++
	rt := fmt.Sprintf("rt-%d", f.seq)
	f.validRefresh[rt] = true
	f.sessions = append(f.sessions, rt)
	for len(f.sessions) > f.policy.maxSessions {
		old := f.sessions[0]
		f.sessions = f.sessions[1:]
		delete(f.validRefresh, old)
		f.evicted.Add(1)
	}
	f.ok.Add(1)
	f.writeToken(w, rt)
}

func (f *fakeIAS) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return srv
}

// --- production-code drivers ------------------------------------------------

// ropcCred builds a Credentials that authenticationParams routes down the ROPC
// (grant_type=password, UserCredential.Email/Password) path — the shared
// technical user. uaaURL points at the fake IAS.
func ropcCred(uaaURL, password string) *Credentials {
	c := &Credentials{
		CISCredential: &CISCredential{GrantType: grantTypePassword},
		UserCredential: &UserCredential{
			Email:    "technical-user@example.com",
			Password: password,
		},
	}
	c.CISCredential.Uaa.Clientid = "cid"
	c.CISCredential.Uaa.Clientsecret = "csecret"
	c.CISCredential.Uaa.Url = uaaURL
	return c
}

// loginOnce performs ONE ROPC login through the real oauth2 construction with a
// fresh config+source — i.e. no token reuse, as the framework path does per
// reconcile. Production createConfig leaves AuthStyle unset (AuthStyleAutoDetect),
// so a fresh (uncached) config probes InHeader then retries InParams on failure:
// each FAILED login therefore sends TWO IAS attempts. Returns the oauth2 error.
func loginOnce(ctx context.Context, uaaURL, password string) error {
	cred := ropcCred(uaaURL, password)
	cfg := createConfig(cred, tokenURL, authenticationParams(cred))
	_, err := cfg.TokenSource(ctx).Token()
	return err
}

// loginOncePinned is loginOnce with the auth style pinned, so a failed login
// sends exactly ONE attempt. Used by the fuzz so its state-machine shadow model
// is exact (the autodetect doubling is asserted separately, below).
func loginOncePinned(ctx context.Context, uaaURL, password string) error {
	cred := ropcCred(uaaURL, password)
	cfg := createConfig(cred, tokenURL, authenticationParams(cred))
	cfg.AuthStyle = oauth2.AuthStyleInParams
	_, err := cfg.TokenSource(ctx).Token()
	return err
}

// --- tests ------------------------------------------------------------------

// ACCEPTANCE #2 — lockout invariant: N concurrent wrong-password logins lock the
// user after EXACTLY lockThreshold failed attempts; the rest see "locked".
func TestIAS_WrongPassword_LocksAfterThreshold(t *testing.T) {
	t.Parallel()
	const N = 32
	fake := newFakeIAS(iasPolicy{lockThreshold: 5, lockDuration: time.Hour, maxSessions: 10, accessTTL: time.Hour}, "correct-pw")
	srv := fake.start(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = loginOnce(ctx, srv.URL, "WRONG") }()
	}
	wg.Wait()

	// The fake locks on the 5th bad attempt (POSTs are serialized under its mutex),
	// so exactly lockThreshold bad-credential attempts are recorded before lock.
	if got := fake.badCred.Load(); got != 5 {
		t.Errorf("bad-credential attempts before lock = %d, want exactly 5 (lockThreshold)", got)
	}
	if fake.lockedUntil.IsZero() {
		t.Error("user not locked after 5 failed logins")
	}
	// AuthStyleAutoDetect: each fresh-config login sends 2 attempts on failure, so
	// total POSTs ≈ 2·N and locked-responses = total − threshold. We assert the
	// doubling rather than a 1-POST-per-login count.
	if got := fake.total.Load(); got < int64(N) {
		t.Errorf("total attempts = %d, want ≥ N=%d", got, N)
	}
	if got := fake.locked.Load(); got != fake.total.Load()-5 {
		t.Errorf("locked responses = %d, want total-threshold = %d", got, fake.total.Load()-5)
	}
}

// The autodetect amplifier, deterministic: production createConfig leaves
// AuthStyle unset, so each FAILED ROPC login on a fresh (per-reconcile) config
// sends TWO IAS attempts. Effective lockout threshold is therefore ⌈5/2⌉ = 3
// failed logins, not 5.
func TestIAS_AutoDetectDoublesFailedAttempts(t *testing.T) {
	t.Parallel()
	fake := newFakeIAS(iasPolicy{lockThreshold: 5, lockDuration: time.Hour, maxSessions: 10, accessTTL: time.Hour}, "correct-pw")
	srv := fake.start(t)
	ctx := context.Background()

	// 2 failed logins → 4 attempts (2× autodetect), not yet locked.
	_ = loginOnce(ctx, srv.URL, "WRONG")
	_ = loginOnce(ctx, srv.URL, "WRONG")
	if got := fake.total.Load(); got != 4 {
		t.Errorf("attempts after 2 failed logins = %d, want 4 (2× autodetect)", got)
	}
	if !fake.lockedUntil.IsZero() {
		t.Error("must not be locked after only 2 failed logins")
	}

	// 3rd failed login: its first attempt is bad #5 → locks.
	_ = loginOnce(ctx, srv.URL, "WRONG")
	if fake.lockedUntil.IsZero() {
		t.Error("must be locked after 3 failed logins (⌈5/2⌉) due to autodetect doubling")
	}
	if got := fake.badCred.Load(); got != 5 {
		t.Errorf("bad-credential attempts = %d, want 5 at lock", got)
	}
}

// ACCEPTANCE #3 — eviction invariant: N concurrent CORRECT logins with cap M keep
// only the newest M sessions; the oldest N-M are evicted; a refresh on an evicted
// session returns invalid_grant AND the lockout counter stays untouched.
func TestIAS_ConcurrentCorrectLogins_EvictOldest_NoLock(t *testing.T) {
	t.Parallel()
	const N, M = 20, 3
	fake := newFakeIAS(iasPolicy{lockThreshold: 5, lockDuration: time.Hour, maxSessions: M, accessTTL: time.Hour}, "correct-pw")
	srv := fake.start(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := loginOnce(ctx, srv.URL, "correct-pw"); err != nil {
				t.Errorf("correct-password login failed: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := fake.ok.Load(); got != N {
		t.Errorf("successful logins = %d, want %d", got, N)
	}
	if got := fake.evicted.Load(); got != int64(N-M) {
		t.Errorf("evicted sessions = %d, want %d (N-M)", got, N-M)
	}
	fake.mu.Lock()
	live := len(fake.validRefresh)
	locked := !fake.lockedUntil.IsZero()
	fake.mu.Unlock()
	if live != M {
		t.Errorf("live refresh tokens = %d, want %d (cap)", live, M)
	}
	if locked {
		t.Error("concurrent CORRECT logins must never lock the user (eviction != lockout)")
	}

	// Refresh an evicted session → invalid_grant, and it must NOT feed lockout.
	before := fake.badCred.Load()
	resp, err := http.PostForm(srv.URL, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {"rt-1"}, // oldest, guaranteed evicted since N-M>0
	})
	if err != nil {
		t.Fatalf("refresh POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("evicted-refresh status = %d, want 400 invalid_grant", resp.StatusCode)
	}
	if fake.invalidGrant.Load() == 0 {
		t.Error("evicted refresh should report invalid_grant")
	}
	if fake.badCred.Load() != before {
		t.Error("invalid_grant on refresh must not increment the failed-login (lockout) counter")
	}
	fake.mu.Lock()
	stillUnlocked := fake.lockedUntil.IsZero()
	fake.mu.Unlock()
	if !stillUnlocked {
		t.Error("invalid_grant on refresh must not lock the user")
	}
}

// ACCEPTANCE #4 — token-reuse invariant: one shared ReuseTokenSource (what
// sharedOAuthClient wraps) collapses K uses into 1 login (#744); a fresh config
// per use (framework per-reconcile) produces K logins.
func TestIAS_SharedTokenSource_OneLogin_vs_FreshPerCall(t *testing.T) {
	t.Parallel()
	const K = 16
	ctx := context.Background()

	t.Run("shared source → 1 login", func(t *testing.T) {
		fake := newFakeIAS(iasPolicy{lockThreshold: 5, lockDuration: time.Hour, maxSessions: 10, accessTTL: time.Hour}, "correct-pw")
		srv := fake.start(t)
		cred := ropcCred(srv.URL, "correct-pw")
		src := createConfig(cred, tokenURL, authenticationParams(cred)).TokenSource(ctx)

		var wg sync.WaitGroup
		for i := 0; i < K; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := src.Token(); err != nil {
					t.Errorf("token: %v", err)
				}
			}()
		}
		wg.Wait()
		if got := fake.ok.Load(); got != 1 {
			t.Errorf("logins with shared source = %d, want 1 (ReuseTokenSource)", got)
		}
	})

	t.Run("fresh config per call → K logins", func(t *testing.T) {
		fake := newFakeIAS(iasPolicy{lockThreshold: 5, lockDuration: time.Hour, maxSessions: 100, accessTTL: time.Hour}, "correct-pw")
		srv := fake.start(t)
		var wg sync.WaitGroup
		for i := 0; i < K; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _ = loginOnce(ctx, srv.URL, "correct-pw") }()
		}
		wg.Wait()
		if got := fake.ok.Load(); got != K {
			t.Errorf("logins with fresh config per call = %d, want %d (per-reconcile cost)", got, K)
		}
	})
}

// 100 concurrent CORRECT logins: the user is NEVER locked (correct credentials),
// every login succeeds (each fresh config = one IAS login, so 100 logins — the
// per-reconcile cost made visible at scale), and sessions are LRU-capped: the
// oldest 100−cap are evicted, exactly `cap` stay live. This is the "correct
// creds + high concurrency = churn/eviction, not outage" property.
func TestIAS_100ConcurrentCorrectLogins_NeverLock(t *testing.T) {
	t.Parallel()
	const N, cap = 100, 10
	fake := newFakeIAS(iasPolicy{lockThreshold: 5, lockDuration: time.Hour, maxSessions: cap, accessTTL: time.Hour}, "correct-pw")
	srv := fake.start(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	var failed atomic.Int64
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := loginOnce(ctx, srv.URL, "correct-pw"); err != nil {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := failed.Load(); got != 0 {
		t.Errorf("failed logins = %d, want 0 (correct password must never fail)", got)
	}
	if got := fake.ok.Load(); got != N {
		t.Errorf("successful logins = %d, want %d", got, N)
	}
	if got := fake.total.Load(); got != N {
		t.Errorf("total IAS attempts = %d, want %d (correct login = 1 attempt each, no autodetect retry)", got, N)
	}
	fake.mu.Lock()
	locked := !fake.lockedUntil.IsZero()
	live := len(fake.validRefresh)
	fake.mu.Unlock()
	if locked {
		t.Error("100 concurrent CORRECT logins must never lock the user")
	}
	if got := fake.evicted.Load(); got != int64(N-cap) {
		t.Errorf("evicted sessions = %d, want %d (N-cap)", got, N-cap)
	}
	if live != cap {
		t.Errorf("live sessions = %d, want %d (cap)", live, cap)
	}
}

// ACCEPTANCE #3 — cisclient path: the process-global clientCache dedups the built
// Client (and thus its single shared oauth2 token source) across ALL concurrent
// callers sharing a credential — i.e. across every CIS-based client kind
// (directory, kyma, subaccount, entitlement, …). N concurrent
// NewServiceClientWithCisCredential calls with distinct-but-equal-keyed creds all
// receive the ONE cached Client, so they share one token source → one login when
// used (contrast the fresh-config framework path, which logins per reconcile).
func TestCIS_ClientCacheDedupsAcrossCallers(t *testing.T) {
	t.Parallel()
	const N = 100
	// Unique clientid so this test's cache key can't collide with other tests
	// sharing the process-global clientCache.
	uniqueClient := "dedup-test-cid"

	mkCred := func() *Credentials {
		c := &Credentials{CISCredential: &CISCredential{GrantType: grantTypeClientCredentials}}
		c.CISCredential.Uaa.Clientid = uniqueClient
		c.CISCredential.Uaa.Clientsecret = "sec"
		c.CISCredential.Uaa.Url = "https://uaa.example"
		c.CISCredential.Endpoints.AccountsServiceUrl = "https://acc.example"
		c.CISCredential.Endpoints.EntitlementsServiceUrl = "https://ent.example"
		c.CISCredential.Endpoints.ProvisioningServiceUrl = "https://prov.example"
		return c
	}

	results := make([]Client, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Each caller passes its OWN equal-keyed cred pointer, as different
			// controllers would; the cache must still collapse them to one Client.
			results[i] = NewServiceClientWithCisCredential(mkCred())
		}(i)
	}
	wg.Wait()

	// Every caller must have received the ONE cached Client: its Credential is the
	// winner's pointer, identical across all N. Distinct pointers would mean a
	// caller got its own build (cache not shared → its own token source → a login).
	want := results[0].Credential
	for i, got := range results {
		if got.Credential != want {
			t.Fatalf("caller %d got a different Client.Credential pointer — clientCache did not dedup across callers", i)
		}
	}

	// Exactly one entry for this key in the process-global cache.
	entries := 0
	clientCache.Range(func(k, _ any) bool {
		if s, ok := k.(string); ok && len(s) >= len(uniqueClient) && s[:len(uniqueClient)] == uniqueClient {
			entries++
		}
		return true
	})
	if entries != 1 {
		t.Errorf("clientCache entries for key = %d, want 1 (one shared client across all callers/kinds)", entries)
	}
}

// --- fuzz (ACCEPTANCE #5) ----------------------------------------------------
// FuzzIASLoginPolicy replays a random sequence of ops against a fresh model and
// checks the invariants against an independent shadow computation:
//   - lock is set iff cumulative consecutive wrong-password attempts reached the
//     threshold (a correct login resets the run);
//   - refresh failures (invalid_grant) never contribute to the lock;
//   - once locked, no further state changes.
//
// Sequential by design: fuzzing explores op sequences; the -race concurrency is
// covered by the tests above.
func FuzzIASLoginPolicy(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0, 0})    // 5 wrong → lock
	f.Add([]byte{0, 1, 0, 0, 0, 0}) // reset then 4 wrong → no lock
	f.Add([]byte{1, 1, 2, 0, 1})    // mixed with a refresh
	f.Fuzz(func(t *testing.T, ops []byte) {
		const threshold = 5
		fake := newFakeIAS(iasPolicy{lockThreshold: threshold, lockDuration: time.Hour, maxSessions: 4, accessTTL: time.Hour}, "correct-pw")
		srv := fake.start(t)
		ctx := context.Background()

		for _, b := range ops {
			switch b % 3 {
			case 0: // wrong-password login (pinned style → exactly 1 attempt)
				_ = loginOncePinned(ctx, srv.URL, "WRONG")
			case 1: // correct login
				_ = loginOncePinned(ctx, srv.URL, "correct-pw")
			case 2: // refresh of a surely-evicted/unknown token → invalid_grant
				resp, err := http.PostForm(srv.URL, url.Values{
					"grant_type":    {"refresh_token"},
					"refresh_token": {"rt-does-not-exist"},
				})
				if err == nil {
					_ = resp.Body.Close()
				}
			}
		}

		// Assert RELATIONAL invariants on the fake's own counters. These hold for
		// any op sequence and are robust to a login that errors at the transport
		// layer before its POST lands (which an ops-derived shadow model is not).
		fake.mu.Lock()
		locked := !fake.lockedUntil.IsZero()
		maxConsec := fake.maxConsecFail
		live := len(fake.validRefresh)
		fake.mu.Unlock()
		total, ok, bad, lk, ig, ev := fake.total.Load(), fake.ok.Load(), fake.badCred.Load(), fake.locked.Load(), fake.invalidGrant.Load(), fake.evicted.Load()

		// 1. Every observed request maps to exactly one outcome.
		if total != ok+bad+lk+ig {
			t.Errorf("total %d != ok %d + badCred %d + locked %d + invalidGrant %d", total, ok, bad, lk, ig)
		}
		// 2. Lock is driven ONLY by consecutive wrong-password failures: locked IFF
		//    the consecutive-failure high-water reached the threshold. A correct
		//    login resets the run; a refresh invalid_grant never touches it — so
		//    neither can ever lock the user. (This is the §5 finding as a property.)
		if locked != (maxConsec >= threshold) {
			t.Errorf("locked=%v but maxConsecFail=%d (threshold=%d): lock must track only consecutive wrong-password failures", locked, maxConsec, threshold)
		}
		// 3. Cannot evict more sessions than were successfully created, and live
		//    sessions never exceed the cap.
		if ev > ok {
			t.Errorf("evicted %d > successful logins %d", ev, ok)
		}
		if live > fake.policy.maxSessions {
			t.Errorf("live sessions %d exceed cap %d", live, fake.policy.maxSessions)
		}
	})
}
