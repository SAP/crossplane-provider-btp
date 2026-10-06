package tfclient

// Model-based concurrency test for the no-fork framework auth path (path 3 of 3
// in docs/development/concurrency-and-lockout-analysis.md).
//
// upjet's TerraformPluginFrameworkConnector.Connect calls configureProvider on
// EVERY reconcile with no cache guard: it wraps the (singleton) framework provider
// in a fresh protocol-v5 server and calls ConfigureProvider, which for the
// userPasswordFlow makes terraform-provider-btp POST a ROPC login to the btp-CLI
// server (/login/<ver>). The auth token is NOT reused across reconciles.
//
// This test replicates that exact configureProvider dance over the REAL
// tfprovider.New() against a fake btp-CLI server (login endpoint + 5-strike
// lockout model), proving: N concurrent reconciles → N logins (no reuse), and a
// wrong credential locks the shared technical user. btpcli is an internal package
// so it cannot be called directly; driving the provider's public Configure is the
// faithful seam.

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	tfprovider "github.com/SAP/terraform-provider-btp/btp/provider"
)

// 100 concurrent no-fork reconciles with the CORRECT password: each Connect
// re-configures the provider → a fresh login, so 100 logins hit the btp-CLI
// server (no reuse across reconciles), and the user is never locked.
func TestNoFork_100ConcurrentCorrectLogins_NeverLock(t *testing.T) {
	t.Parallel()
	const N = 100
	fake := &fakeBTPCLI{password: "correct-pw", lockThreshold: 5}
	srv := fake.start(t)
	p := tfprovider.New() // singleton, as tfclient.frameworkProvider() is
	ctx := context.Background()

	var wg sync.WaitGroup
	var failed atomic.Int64
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := noForkConfigureOnce(ctx, p, srv.URL, "correct-pw"); err != nil {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := failed.Load(); got != 0 {
		t.Errorf("failed configures = %d, want 0 (correct password)", got)
	}
	if got := fake.ok.Load(); got != N {
		t.Errorf("btp-CLI logins = %d, want %d (one per reconcile, no reuse)", got, N)
	}
	if got := fake.logins.Load(); got != N {
		t.Errorf("total login POSTs = %d, want %d", got, N)
	}
	fake.mu.Lock()
	locked := fake.locked
	fake.mu.Unlock()
	if locked {
		t.Error("100 concurrent CORRECT no-fork logins must never lock the user")
	}
}

// Wrong password under concurrent no-fork reconciles locks the technical user.
func TestNoFork_WrongPassword_LocksAfterThreshold(t *testing.T) {
	t.Parallel()
	const N = 32
	fake := &fakeBTPCLI{password: "correct-pw", lockThreshold: 5}
	srv := fake.start(t)
	p := tfprovider.New()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = noForkConfigureOnce(ctx, p, srv.URL, "WRONG") }()
	}
	wg.Wait()

	fake.mu.Lock()
	locked, maxConsec := fake.locked, fake.maxConsecFail
	fake.mu.Unlock()
	if !locked {
		t.Error("wrong password under concurrent no-fork reconciles must lock the user")
	}
	if maxConsec < fake.lockThreshold {
		t.Errorf("max consecutive failures = %d, want ≥ threshold %d", maxConsec, fake.lockThreshold)
	}
}
