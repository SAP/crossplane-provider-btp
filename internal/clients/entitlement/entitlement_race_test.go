package entitlement

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	entclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
)

// clearDescribeCache empties the package-level describeCache between probes.
func clearDescribeCache() {
	describeCache.Range(func(k, _ any) bool {
		describeCache.Delete(k)
		return true
	})
}

// TestDescribeCacheGet_ExpiryDeleteRace probes CTL-1.
//
// describeCacheGet, on encountering an expired entry, calls
// describeCache.Delete(key). If a concurrent goroutine has just stored a
// fresh entry for the same key (post-expiry, via fetchAssignments), the
// Delete wipes the fresh value — next reader has to re-fetch, defeating
// the cache. The fix is CompareAndDelete(key, expiredEntry).
//
// We reproduce by pre-loading an expired entry, then racing:
//
//   goroutine E → describeCacheGet(key)     // sees expired, wants to Delete
//   goroutine S → describeCache.Store(fresh) // stores fresh entry
//
// After both complete, the cache should hold the fresh entry. Under the
// current code, ordering "E reads → S stores → E deletes" wipes fresh.
func TestDescribeCacheGet_ExpiryDeleteRace(t *testing.T) {
	// Shares package-level describeCache — do not run in parallel with
	// sibling probes that also mutate it.
	r := require.New(t)

	const iters = 5000
	wipes := 0

	for range iters {
		clearDescribeCache()

		key := "sa|svc|plan"
		expired := &describeEntry{
			val: &entclient.EntitledAndAssignedServicesResponseObject{},
			at:  time.Now().Add(-2 * describeCacheT),
		}
		fresh := &describeEntry{
			val: &entclient.EntitledAndAssignedServicesResponseObject{},
			at:  time.Now(),
		}
		describeCache.Store(key, expired)

		// Pair many racers to widen the interleaving window per iter.
		var wg sync.WaitGroup
		start := make(chan struct{})
		for range 4 {
			wg.Go(func() {
				<-start
				_ = describeCacheGet(key)
			})
			wg.Go(func() {
				<-start
				describeCache.Store(key, fresh)
			})
		}
		close(start)
		wg.Wait()

		got, ok := describeCache.Load(key)
		if !ok || got.(*describeEntry) != fresh {
			wipes++
		}
	}

	t.Logf("fresh-entry wipes: %d / %d iterations", wipes, iters)
	r.Zero(wipes, "describeCacheGet wiped a concurrently-stored fresh entry — use CompareAndDelete(key, expiredEntry)")
}

// TestDescribeCache_UnboundedGrowth probes CTL-2.
//
// Once a key is Stored with an entry that is never Loaded again, the
// entry lives forever. This asserts current behavior: after N unique
// keys are stored and TTLs expire, the map still holds all N. This test
// documents the leak; fixing CTL-2 (background janitor / lazy sweep on
// Store) would let us assert cleanup, which flips this test to Zero().
func TestDescribeCache_UnboundedGrowth(t *testing.T) {
	// Shares package-level describeCache — non-parallel.
	r := require.New(t)

	clearDescribeCache()

	const N = 200
	for i := range N {
		describeCache.Store(
			// unique key per iteration → simulates churn on
			// (subaccount, service, plan) tuples
			"leak-key-"+strconv.Itoa(i),
			&describeEntry{
				val: &entclient.EntitledAndAssignedServicesResponseObject{},
				at:  time.Now().Add(-2 * describeCacheT), // already expired
			},
		)
	}

	// Fire the sweeper directly — janitor goroutine would tick every
	// describeCacheT (30s), too long for a test. describeSweep is
	// exported to the test package to allow deterministic verification.
	describeSweep(time.Now())

	var count int
	describeCache.Range(func(k, _ any) bool {
		count++
		return true
	})

	t.Logf("cache size after N=%d expired stores + sweep: %d", N, count)
	r.Zero(count, "sweeper must evict all expired entries — see CTL-2")
}

// TestFetchAssignments_SingleFlightDedupes probes the happy path of
// fetchAssignments — a regression guard for the singleflight+cache pipe.
// Not a bug probe; ensures future changes don't break the dedup guarantee.
func TestFetchAssignments_SingleFlightDedupes(t *testing.T) {
	// Shares package-level describeCache/describeGroup — non-parallel.
	r := require.New(t)

	clearDescribeCache()

	// Prime the cache with a fresh entry so Do's inner func doesn't hit
	// the network. All concurrent Do calls should observe the cached hit
	// and return without executing the fetch closure.
	key := "sa|svc|plan"
	describeCache.Store(key, &describeEntry{
		val: &entclient.EntitledAndAssignedServicesResponseObject{},
		at:  time.Now(),
	})

	var fetches atomic.Uint64
	// Wrap describeGroup manually — we can't easily call fetchAssignments
	// without a full btp.Client. Exercise singleflight+cache directly.
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			_, _, _ = describeGroup.Do(key, func() (any, error) {
				if cached := describeCacheGet(key); cached != nil {
					return cached, nil
				}
				fetches.Add(1)
				return &entclient.EntitledAndAssignedServicesResponseObject{}, nil
			})
		})
	}
	wg.Wait()

	r.Zero(fetches.Load(), "cache hit must short-circuit the fetch closure for every caller")
}
