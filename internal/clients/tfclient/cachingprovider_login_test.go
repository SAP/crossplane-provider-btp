package tfclient

// Login-count test over the real tfprovider.New() wrapped in newCachingProvider,
// as opposed to the stubProvider in cachingprovider_test.go.

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	tfprovider "github.com/SAP/terraform-provider-btp/btp/provider"
)

// N concurrent reconciles through the caching wrapper hit the login endpoint
// once; the rest replay the cached session.
func TestCachingWrapper_ConcurrentReconciles_OneLogin(t *testing.T) {
	t.Parallel()
	const N = 100
	fake := &fakeBTPCLI{password: "correct-pw"}
	srv := fake.start(t)
	p := newCachingProvider(tfprovider.New())
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
	if got := fake.logins.Load(); got != 1 {
		t.Errorf("btp-CLI logins = %d, want 1 (cache collapses %d reconciles)", got, N)
	}
}

// Two credential sets are cached separately: each logs in once, so repeated
// reconciles for both users produce exactly two logins.
func TestCachingWrapper_TwoCredentials_TwoLogins(t *testing.T) {
	t.Parallel()
	const N = 50
	fake := &fakeBTPCLI{password: "correct-pw"}
	srv := fake.start(t)
	p := newCachingProvider(tfprovider.New())
	ctx := context.Background()

	var wg sync.WaitGroup
	var failed atomic.Int64
	for i := 0; i < N; i++ {
		wg.Add(1)
		user := "user-a"
		if i%2 == 1 {
			user = "user-b"
		}
		go func() {
			defer wg.Done()
			if err := noForkConfigureAs(ctx, p, srv.URL, user, "correct-pw"); err != nil {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := failed.Load(); got != 0 {
		t.Errorf("failed configures = %d, want 0 (correct password)", got)
	}
	if got := fake.logins.Load(); got != 2 {
		t.Errorf("btp-CLI logins = %d, want 2 (one per credential set)", got)
	}
}
