# Client and Read Caching

## Context

The entitlement reconcile path used to issue ~3 HTTP calls per CR per
reconcile to BTP. One GET to the entitlements service, plus roughly two
POSTs to the UAA token endpoint. The providerconfig connector rebuilt the
whole `btp.Client` every reconcile and threw the oauth2 token cache away
with it. At ~40 CRs on the default poll, that ate ~50% of worker time
and ~50 MB/min of bandwidth.

This note covers the two caches that fix it.

## What we cache

### 1. `btp.Client` per credential bundle

In `btp/cisclient.go`: `clientCache`, `clientBuildGroup`,
`credentialCacheKey`, `NewServiceClientWithCisCredential`.

A process-wide `sync.Map` keyed by a NUL-joined string of the credential
fields (UAA clientid, secret, URL, the three service endpoints, grant
type, plus user credential fields when set). The cached value is the
fully-built `btp.Client`. Its three sub-clients (accounts, entitlements,
provisioning) share a single oauth2 `*http.Client` via `sharedOAuthClient`,
so one `oauth2.ReuseTokenSource` covers all three.

Concurrent first-callers on the same key are collapsed into a single
`createClient()` build via `golang.org/x/sync/singleflight.Group`
(`clientBuildGroup`). Plain Load+LoadOrStore would let N callers each
build a Client and discard N-1; singleflight makes it exactly one.

Net effect: we fetch a UAA token once per credential bundle per process.
It refreshes only when the cached `*oauth2.Token` actually expires
(~12h on BTP UAA).

### 2. Entitlement describe results per `(subaccount, service, plan)`

In `internal/clients/entitlement/entitlement.go`: `describeGroup`,
`describeCache`, `describeCacheT`, `fetchAssignments`, `describeSweep`,
`startDescribeSweeper`.

A [`golang.org/x/sync/singleflight.Group`](https://pkg.go.dev/golang.org/x/sync/singleflight#Group)
plus a `sync.Map` with TTL `describeCacheT = 30 * time.Second`. Key:
`subaccountGUID + "|" + serviceName + "|" + planName`.

Singleflight dedupes concurrent sibling reconciles. The TTL absorbs the
back-to-back fan-out across all CRs that share a key in one poll tick.
Writes via `UpdateInstance` (the `SetServicePlans` PUT) invalidate their
own key so the next Observe reads fresh state.

Expired entries are removed via `describeCache.CompareAndDelete(key, entry)`,
not `Delete(key)` — a plain `Delete` would race with a concurrent `Store`
of a fresh entry and wipe it.

A background janitor goroutine, started lazily on first Store via
`sync.Once` (`startDescribeSweeper`), ticks every `describeCacheT` and
sweeps expired entries. Without it, keys that are never re-Loaded would
live for the process lifetime.

## BTP APIs affected

| Endpoint | Effect |
| --- | --- |
| `POST <uaa>/oauth/token` | Cached implicitly via the shared `oauth2.ReuseTokenSource`. Drops from hundreds/min to ~1 per credential per token lifetime. |
| `GET /entitlements/v1/assignments` | Cached for up to `describeCacheT` (30s) per `(subaccount, service, plan)`. |
| `PUT /entitlements/v1/subaccountServicePlans` | Not cached. Each successful write invalidates the matching describe-cache key. |

Other BTP endpoints (accounts, provisioning) are not cached at the
application layer, but they still benefit from the shared oauth2 token
cache via `sharedOAuthClient`.

## Caveats

- **Credential rotation leaks entries.** Rotating a secret changes the
  cache key, so the next Connect builds a fresh `btp.Client`. The old one
  sits in `clientCache` until process restart. Fine for monthly-ish
  rotation. Add a TTL or LRU if rotation churn matters.
- **The describe-cache TTL is global.** Don't reuse the cache for an
  endpoint that needs a different freshness window. Add a dedicated
  cache.
- **Caches are process-local.** Each replica keeps its own. BTP is the
  source of truth so this is fine.

## Adding a new BTP read

Decide if a 30s window is acceptable. If yes, route through
`fetchAssignments` (or a sibling helper with the same shape). If no, hit
BTP directly. Add a dedicated short-lived cache only when measured call
volume justifies it.

If you add a new write that mutates state the describe cache observes,
invalidate the matching key via `describeCache.Delete(...)` on success.

## Testing the call-count guarantees

The two claims that motivated this file — one token POST per credential
per token lifetime, and one describe GET per `(subaccount, service,
plan)` per TTL window — are verified end-to-end via
`internal/httpcount.RoundTripper` (a test-only counting `http.RoundTripper`
that records total / per-key / per-host call counts under atomic +
`sync.RWMutex`):

- `btp/cisclient_httpcount_test.go` —
  `TestClientCache_SharedTokenSource_OnePOSTAcrossManyGETs`: 10 GETs
  across sub-clients produce exactly 1 POST `/oauth/token`.
- `internal/clients/entitlement/entitlement_httpcount_test.go` —
  `TestDescribeInstance_CachedWithinTTL` (10 sequential describes → 1
  GET), `TestDescribeInstance_SingleflightConcurrent` (32 concurrent
  describes → 1 GET), `TestDescribeInstance_DistinctKeys_NoSharing`
  (5 distinct keys → 5 GETs), `TestUpdateInstance_InvalidatesDescribeCache`
  (describe → describe → PUT → describe → 2 GETs + 1 PUT).

Concurrency invariants (TTL-delete race, singleflight collapse,
unbounded growth) have their own race probes in `*_race_test.go`
siblings.

Two test seams let tests inject instrumentation without rewiring
production paths:

- `btp/cisclient.go: buildClientFn` — counts `createClient` invocations
  under contention (verifies the `clientBuildGroup` singleflight).
- `btp/cisclient.go: newBaseHTTPClientFn` — swaps the base HTTP client
  behind oauth2 for an `httpcount.RoundTripper` (verifies the shared
  token source).

Add a new counted-call test whenever you add a new cached BTP read.
