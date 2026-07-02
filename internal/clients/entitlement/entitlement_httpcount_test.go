package entitlement

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/httpcount"
	entclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
)

// These tests verify the two claims from PR #744 that concern the
// entitlement client's call volume:
//
//   * "GET /entitlements/v1/assignments" is cached for describeCacheT
//     (30s) per (subaccount, service, plan) key.
//   * Concurrent sibling reconciles on the same key share ONE inflight
//     GET via singleflight.
//   * "UpdateInstance" (PUT SetServicePlans) invalidates the cached key
//     so the next DescribeInstance fires a fresh GET.
//
// The whole real openapi client is wired against an httptest server
// through an httpcount.RoundTripper — the counter observes the actual
// HTTP calls that leave the process, not a mock's Method-called count.

const (
	testSubaccount = "0000-sub"
	testService    = "hana-cloud"
	testPlan       = "hana"
)

// buildEntitlementsClient wires a real ManageAssignedEntitlementsAPIService
// against an httptest server. Returns the client, the counter, and the
// server (test-cleanable via t.Cleanup).
func buildEntitlementsClient(t *testing.T, handler http.HandlerFunc) (EntitlementsClient, *httpcount.RoundTripper, *httptest.Server) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	rt := httpcount.New(nil)
	cfg := entclient.NewConfiguration()
	cfg.HTTPClient = rt.Client()
	cfg.Servers = []entclient.ServerConfiguration{{URL: srv.URL}}
	api := entclient.NewAPIClient(cfg).ManageAssignedEntitlementsAPI

	c := EntitlementsClient{
		btp: btp.Client{EntitlementsServiceClient: api},
	}
	return c, rt, srv
}

// assignmentsHandler returns a handler that responds with a minimal but
// valid EntitledAndAssignedServicesResponseObject and lets tests inspect
// how many times it fired.
func assignmentsHandler(fires *int, mu *sync.Mutex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*fires++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		serviceName := testService
		planName := testPlan
		resp := entclient.EntitledAndAssignedServicesResponseObject{
			AssignedServices: []entclient.AssignedServiceResponseObject{
				{
					Name: &serviceName,
					ServicePlans: []entclient.AssignedServicePlanResponseObject{
						{
							Name: &planName,
						},
					},
				},
			},
			EntitledServices: []entclient.EntitledServicesResponseObject{
				{
					Name: &serviceName,
					ServicePlans: []entclient.ServicePlanResponseObject{
						{
							Name: &planName,
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func newTestCR(service, plan, subaccount string) *v1alpha1.Entitlement {
	cr := &v1alpha1.Entitlement{}
	cr.Spec.ForProvider.ServiceName = service
	cr.Spec.ForProvider.ServicePlanName = plan
	cr.Spec.ForProvider.SubaccountGuid = subaccount
	// UpdateInstance dereferences Status.AtProvider.Required — initialize.
	cr.Status.AtProvider = &v1alpha1.EntitlementObservation{
		Required: &v1alpha1.EntitlementSummary{},
	}
	return cr
}

// TestDescribeInstance_CachedWithinTTL — 10 sequential describes on the
// same key must produce exactly ONE GET on the wire (PR #744 §2: 30s TTL).
func TestDescribeInstance_CachedWithinTTL(t *testing.T) {
	// Shares package-level describeCache — non-parallel.
	r := require.New(t)

	clearDescribeCache()

	var fires int
	var mu sync.Mutex
	client, rt, srv := buildEntitlementsClient(t, assignmentsHandler(&fires, &mu))
	_ = srv

	cr := newTestCR(testService, testPlan, testSubaccount)
	const calls = 10
	for range calls {
		_, err := client.DescribeInstance(context.Background(), cr)
		r.NoError(err)
	}

	got := rt.CountFor("GET", "/entitlements/v1/assignments")
	r.Equal(uint64(1), got, "TTL cache must collapse %d sequential describes to 1 GET; got %d", calls, got)
	r.Equal(1, fires, "server handler must fire once too")
	r.Equal(uint64(1), rt.Total(), "no other endpoints should have been hit")
}

// TestDescribeInstance_SingleflightConcurrent — 32 goroutines all firing
// DescribeInstance at once on the same key must share ONE inflight GET
// (PR #744 §2: singleflight).
func TestDescribeInstance_SingleflightConcurrent(t *testing.T) {
	// Shares package-level describeCache/describeGroup — non-parallel.
	r := require.New(t)

	clearDescribeCache()

	var fires int
	var mu sync.Mutex
	client, rt, _ := buildEntitlementsClient(t, assignmentsHandler(&fires, &mu))

	cr := newTestCR(testService, testPlan, testSubaccount)
	const workers = 32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range workers {
		wg.Go(func() {
			<-start
			_, err := client.DescribeInstance(context.Background(), cr)
			r.NoError(err)
		})
	}
	close(start)
	wg.Wait()

	got := rt.CountFor("GET", "/entitlements/v1/assignments")
	r.Equal(uint64(1), got, "singleflight + TTL cache must collapse %d concurrent describes to 1 GET; got %d", workers, got)
}

// TestDescribeInstance_DistinctKeys_NoSharing — describes on different
// (subaccount, service, plan) keys must NOT share the cache. N distinct
// keys must produce N GETs.
func TestDescribeInstance_DistinctKeys_NoSharing(t *testing.T) {
	// Shares package-level describeCache — non-parallel.
	r := require.New(t)

	clearDescribeCache()

	var fires int
	var mu sync.Mutex
	client, rt, _ := buildEntitlementsClient(t, assignmentsHandler(&fires, &mu))

	for i := range 5 {
		cr := newTestCR(testService, testPlan, fmt.Sprintf("sub-%d", i))
		_, err := client.DescribeInstance(context.Background(), cr)
		r.NoError(err)
	}

	got := rt.CountFor("GET", "/entitlements/v1/assignments")
	r.Equal(uint64(5), got, "distinct keys must not share cache; expected 5 GETs, got %d", got)
}

// TestUpdateInstance_InvalidatesDescribeCache — PR #744 §2 promise:
// after a successful PUT SetServicePlans, the next DescribeInstance must
// hit the network again instead of returning stale cached data.
func TestUpdateInstance_InvalidatesDescribeCache(t *testing.T) {
	// Shares package-level describeCache — non-parallel.
	r := require.New(t)

	clearDescribeCache()

	var describeFires int
	var putFires int
	var mu sync.Mutex

	handler := func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/entitlements/v1/assignments":
			assignmentsHandler(&describeFires, &mu).ServeHTTP(w, r)
		case r.Method == http.MethodPut && r.URL.Path == "/entitlements/v1/subaccountServicePlans":
			mu.Lock()
			putFires++
			mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	}

	client, rt, _ := buildEntitlementsClient(t, handler)

	cr := newTestCR(testService, testPlan, testSubaccount)

	// First describe → 1 GET.
	_, err := client.DescribeInstance(context.Background(), cr)
	r.NoError(err)
	r.Equal(uint64(1), rt.CountFor("GET", "/entitlements/v1/assignments"))

	// Second describe within TTL → still 1 GET (cache hit).
	_, err = client.DescribeInstance(context.Background(), cr)
	r.NoError(err)
	r.Equal(uint64(1), rt.CountFor("GET", "/entitlements/v1/assignments"))

	// PUT SetServicePlans → 1 PUT + invalidates cache key.
	err = client.UpdateInstance(context.Background(), cr)
	r.NoError(err)
	r.Equal(uint64(1), rt.CountFor("PUT", "/entitlements/v1/subaccountServicePlans"))

	// Third describe post-write → forced fresh GET.
	_, err = client.DescribeInstance(context.Background(), cr)
	r.NoError(err)
	r.Equal(uint64(2), rt.CountFor("GET", "/entitlements/v1/assignments"),
		"UpdateInstance must invalidate the describe cache; expected 2 GETs total after write, got %d",
		rt.CountFor("GET", "/entitlements/v1/assignments"))
}
