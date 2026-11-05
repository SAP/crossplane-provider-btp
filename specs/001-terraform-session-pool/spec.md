# Spec 001 — Warmed Terraform BTP Session Pool

**Status:** Draft
**Deprecates (kept + mapped, not removed):** the reuse boolean introduced in #1094
(`--enable-terraform-session-reuse`)
**Owner:** Max Braun

## 1. Context & Problem

The generated upjet controllers and the internal Terraform connectors talk to the
BTP CLI server through the upstream `terraform-provider-btp` client (`btpcli`).

Observed facts from the client source (`terraform-provider-btp` `internal/btpcli`):

- A configured `v2Client` holds exactly **one** session: `v2Client.session *Session`
  (`client.go`). One client = one session.
- The session is **global-account scoped**: `Session{ GlobalAccountSubdomain,
  SessionId, IdentityProvider, LoggedInUser }` (`session.go`). There is no
  subaccount field; subaccount is a per-request body parameter sent over the same
  GA session.
- The serialization lock **is the session itself** — `Session` embeds `sync.Mutex`,
  and `doRequest` does `v2.session.Lock(); defer Unlock()` around the whole HTTP
  round-trip (`client.go`). The lock is therefore GA-scoped.

Consequences of the two modes shipped so far:

- **Shared single client (old default, now `--enable-terraform-session-reuse`):**
  every reconcile serializes on one GA session lock → head-of-line blocking →
  reconciles burn their timeout waiting (the bug #1094 chased).
- **Client-per-reconcile (new default after #1094):** each Configure authenticates
  → login storm, risks BTP concurrent-session limits, orphans sessions (no logout).

Both are endpoints of a single bad trade-off. Neither pools.

## 2. Goals / Non-goals

**Goals**

- Provide the **middle option** between the two extremes shipped so far: a client
  per reconcile (multiple logins per second) on one end, and a single client that
  logs in once but funnels every request through one session mutex on the other. The
  pool sits in between — a few warmed sessions, reused, bounded.
- Replace the reuse boolean with a bounded pool of warmed, reusable GA sessions
  that removes head-of-line blocking **and** bounds login/session count.
- Self-tune the pool size to the controllers' configured concurrency.
- Expire stale sessions (TTL) and drop server-killed sessions (dirty-marking).
- Expose pool, HTTP-connection, and async-operation state on `/metrics`.

**Non-goals**

- Exclusive checkout/checkin (one session per in-flight reconcile). Not achievable
  soundly in this stack — see §4.1.
- Explicit server-side logout of evicted sessions (deferred — see §9).

## 3. Design overview

Per global-account key, maintain a ring of up to **N warmed `btpcli` sessions**,
handed out **round-robin** and **shared** (not exclusively checked out). N shared
sessions → N GA locks → N-way parallelism; concurrent users of one session serialize
on its mutex (safe, bounded). This is the only model consistent with upjet's
lifecycle (§4.1).

### 3.1 Key = global account

Keep #1094's key: `fmt.Sprintf("%v", req.Config.Raw)`, where `Config.Raw` is the GA
provider config (username / globalaccount / idp / cli_server_url). This matches the
session scope exactly (the session is GA-scoped, §1).

### 3.2 Sizing

`N = MaxConcurrentReconciles + 2` (auto), overridable via
`--terraform-session-pool-size`. The `+2` is headroom for eviction/warm churn.

Pool-size value semantics (the single knob spans both old extremes and the new
middle):

- `0` → pool disabled; a fresh client per reconcile (= old `--enable-terraform-session-reuse=false`,
  #1094's default: multiple logins/sec).
- `1` → one shared warmed session; every request funnels through its mutex
  (= old `--enable-terraform-session-reuse=true`).
- `≥2` → the pool (the middle option).
- unset → `auto` = `MaxConcurrentReconciles + 2`.

This requires **unpinning** the Terraform controllers' concurrency: today
`setupTerraformControllers` hard-codes `MaxConcurrentReconciles: 1`
(`cmd/provider/main.go`), because a shared single lock made parallel workers
pointless. The pool is what makes `>1` worthwhile, so TF concurrency becomes
`*maxReconcileRate` (matching the native controllers).

> Note: `maxReconcileRate` feeds two **rate** limiters (REST config +
> `GlobalRateLimiter`); the only **concurrency** cap is `MaxConcurrentReconciles`.
> N tracks that cap. Real concurrent demand on one key = `MaxConcurrentReconciles ×
> (#TF types sharing the key)`; demand above N degrades gracefully by sharing
> existing sessions (today's behavior), never by unbounded session growth.

### 3.3 Slot lifecycle

Per-slot state: `{ client, sessionID, createdAt, dirty atomic.Bool }`.

1. **Warm (lazy):** first checkout of a slot performs the ROPC login; the session
   stays warm for reuse. No eager startup pre-warm (avoids a login burst for configs
   that never reconcile).
2. **Reuse:** later checkouts return the warm session.
3. **Dirty:** `cliTransport` (`terraformprovider.go`) on a 401 carrying session
   headers (`X-Cpcli-Sessionid`) marks that slot's client `dirty`. This replaces the
   current nuke-by-subdomain eviction with a per-slot flag.
4. **TTL:** `now - createdAt > ttl` at checkout → slot is stale. Default **2h**,
   overridable via `--terraform-session-ttl`.
5. **Reclaim = GC-only, no logout:** dirty or stale slots are skipped on handout and
   a fresh client is warmed into the slot. In-flight goroutines keep their own
   reference and run to completion; the old client is GC'd afterward. Never log out
   on eviction — that would kill a session a background (async) op is mid-call on.

### 3.4 Shared HTTP transport

All N sessions ride **one shared `http.Client`** (`newBTPFrameworkProvider` builds
one `hc` for `tfprovider.NewWithClient`; every `v2Client` uses it) → one TCP
connection pool. `http.DefaultTransport`'s `MaxIdleConnsPerHost` is **2**, which
would cap keep-alive reuse below the session concurrency and force per-call TLS
handshakes. Clone DefaultTransport (do not mutate the global) and set
`MaxIdleConnsPerHost`/`MaxIdleConns ≈ N`.

## 4. Why this shape (key decisions)

### 4.1 No exclusive checkout — upjet has no usable check-in

- crossplane-runtime calls `external.Disconnect(ctx)` every reconcile
  (`managed/reconciler.go`), **but** upjet's framework external client stubs
  `Disconnect` to `return nil` (`external_tfpluginfw.go`) and never calls back into
  the provider.
- Generated controllers build the connecter **inline**
  (`zz_controller.go: managed.WithExternalConnecter(NewTerraformPluginFrameworkAsyncConnector(...))`),
  so wrapping `Disconnect` is not reachable without a codegen-template change.
- The **async** connector runs the TF op in a goroutine that outlives
  Connect/Disconnect (`external_async_tfpluginfw.go` + OperationStore). An exclusive
  pool that reclaims the session on Disconnect would hand a still-in-use session to a
  second reconcile → unsound.

Shared-ring sidesteps all three: there is no reclaim event to get wrong; concurrent
use is the defined, mutex-safe path. Contention drops to ~1/N (not zero), which is
the actual fix.

### 4.2 Async operation count is not a problem

Async ops draw from the ring; they never mint their own session. K concurrent ops
map onto N sessions by round-robin (sharing is mutex-safe). The session lock is held
per CLI round-trip, not per operation, so a long apply does not pin a session. The
one bounded edge: an in-flight op holding a slot that then gets evicted keeps that
old client alive (GC-only), so during eviction churn live sessions can transiently
exceed N by the number of in-flight ops on evicted clients. Bounded, rare,
self-healing; observable via the `evicted_inflight` gauge (§5).

## 5. Metrics (`/metrics`, controller-runtime registry)

Registered into `sigs.k8s.io/controller-runtime/pkg/metrics` `Registry` via
`promauto`. Promote `github.com/prometheus/client_golang` to a direct dependency.

### 5.1 Session pool — label `global_account`

- gauge `btp_tf_session_pool_size` — configured N.
- gauge `btp_tf_session_pool_dirty` — currently dirty-flagged (normally 0).
- counter `btp_tf_session_pool_logins_total` — warms performed (the thing we minimize).
- counter `btp_tf_session_pool_checkouts_total` — reuses of a warm slot. Reuse ratio =
  checkouts/logins.
- counter `btp_tf_session_pool_evictions_total{reason="dirty|ttl"}` — churn by cause.

Deferred (`ponytail:`): a `warm` gauge (live-usable count — fuzzy to keep as a running
value) and `evicted_inflight` (needs in-flight-op tracking, §4.2). Add if the ceiling
needs direct observation.

### 5.2 HTTP connection — at `cliTransport`

- counter `btp_tf_cli_http_requests_total{code,method}` (`promhttp`).
- histogram `btp_tf_cli_http_request_duration_seconds{code,method}`.
- gauge `btp_tf_cli_http_in_flight`.
- counter `btp_tf_cli_http_connections_total{reused="true|false"}` (via `httptrace`
  `GotConnInfo`) — verifies keep-alive reuse / the `MaxIdleConnsPerHost` tuning
  (§3.4).

No `global_account` label — the transport is shared across GAs. Optional `host`
label only if multiple CLI servers are in use (deferred).

### 5.3 Async operations — at `tf_async_callback.go`

- counter `btp_tf_async_callbacks_total{op="create|update|destroy",result="success|error"}`
  — incremented in `APICallbacks.callback` (we own this seam; `err` gives result).

In-flight async gauge is deferred: the op **start** is inside upjet and only
reachable on the internal-connector path, not for generated controllers.
upjet already exposes `ttr`, `reconcile_delay`, and `active_cli_invocations`
(≈0 in the no-fork path).

## 6. Config knobs (net change)

- **added:** `--terraform-session-pool-size` (default `auto` = MaxConcurrentReconciles+2;
  `0`/`1` reproduce the two old extremes — see §3.2). Non-boolean so it can evolve.
- **added:** `--terraform-session-ttl` (default `2h`).
- **deprecated (kept, backwards compatible):** `--enable-terraform-session-reuse` /
  `ENABLE_TERRAFORM_SESSION_REUSE`. Still parses; logs a deprecation warning pointing
  at `--terraform-session-pool-size`.
- **changed:** Terraform controllers' `MaxConcurrentReconciles` `1` → `*maxReconcileRate`.

### Precedence / mapping

- If `--terraform-session-pool-size` is set explicitly, it wins; the deprecated flag
  is ignored (deprecation warning if both set).
- Else if `--enable-terraform-session-reuse` is set explicitly: `true` → pool-size `1`,
  `false` → pool-size `0`. Exact old behavior preserved.
- Else (neither set): `auto`.

So an existing deployment passing the old flag keeps its exact behavior; a deployment
passing nothing moves to the auto pool (an intentional default improvement — pin
`--terraform-session-pool-size=0` or `--enable-terraform-session-reuse=false` to keep
#1094's per-reconcile default).

## 7. Acceptance criteria (pass/fail)

1. TF controllers run at `MaxConcurrentReconciles = *maxReconcileRate`
   (`cmd/provider/main.go`); native unchanged.
2. `--terraform-session-pool-size` (auto default = MaxConcurrentReconciles+2) and
   `--terraform-session-ttl` (2h) exist and appear in `--help`.
   `--enable-terraform-session-reuse` is **kept, deprecated** — still parses, logs a
   deprecation warning, and maps per §6: unset→auto, `=true`→size 1, `=false`→size 0.
   `--terraform-session-pool-size` wins when both are set.
3. `M ≫ N` concurrent `Configure` calls on one GA key → exactly **N** logins
   (fake btpcli `ok == N`); N distinct sessions handed round-robin.
4. In-flight peak == `min(M, N)` via an injected fake inner provider on the
   `newTerraformProvider(inner, ...)` seam — proves N-way parallelism on the data path.
5. A 401 carrying session headers marks only that slot dirty; next checkout re-logins
   (logins N→N+1), other slots untouched. `evictions_total{reason="dirty"}` += 1.
6. A checkout of a client older than TTL discards it and re-logins (injected clock);
   `evictions_total{reason="ttl"}` += 1.
7. `size=1` reproduces single-shared-session behavior and `size=0` reproduces
   per-reconcile fresh clients (= old flag `true`/`false`); the deprecated flag maps to
   these per §6. Wrong password at `size=N` still locks the technical user at the
   5-strike threshold (no lockout amplification).
8. HTTP: `requests_total` increments; with `MaxIdleConnsPerHost ≈ N`,
   `connections_total{reused="true"}` rises across sequential calls (and stays low at
   the default 2 — regression guard on the tuning).
9. Async: `btp_tf_async_callbacks_total` increments with the correct `op`/`result`.
10. Metrics are registered on the controller-runtime registry and scrapeable.
11. `go build ./...` and `go test -race ./internal/clients/tfclient ./cmd/provider`
    pass.

## 8. Testing

Reuse the existing `fakeBTPCLI` harness (`fake_btpcli_test.go`) and the
`noForkConfigure*` seam.

- Pool reuse / login bound / round-robin: drive `M` concurrent `Configure`, assert
  `fake.ok == N` and `logins_total == N`.
- Parallelism: inject a fake inner `fwprovider.Provider` whose Configure returns a
  client with an instrumented RoundTripper + a concurrency gauge; assert peak ==
  `min(M, N)`.
- Dirty: fake returns 401 with session headers on a data call → assert the one slot
  re-warms, others intact.
- TTL: inject `now func() time.Time`; advance past TTL; assert discard + re-login.
- Lockout: wrong password at `size=N` → still locks at threshold.
- Metrics: `prometheus/client_golang/prometheus/testutil` for the counters/gauges.
- HTTP: `httptest` server; assert request + connection-reuse counters.

## 9. Risks & ceilings (`ponytail:` upgrade paths)

- **GC-only reclaim** leaves dead sessions until GC. Fine unless BTP concurrent-session
  quota bites → add a reaper + explicit logout, guarded against in-flight async.
- **TTL vs server session lifetime:** confirm the BTP CLI server session lifetime. If
  it is `< 2h`, TTL never wins the race and the 401/dirty path carries all refreshes →
  lower the TTL default or drop it.
- **Transient over-provisioning** during eviction churn (§4.2). If it threatens a tight
  session cap → track outstanding-evicted and block new warms until it drains.
- **In-flight async gauge** not exposed (start seam not owned for generated controllers)
  → wrap the connecter via a codegen-template change if the gauge is needed.

## 10. Delivery

Single follow-up PR on top of merged #1094 (not a revert): spec + implementation
together, per the workspace development workflow. Spec and code stay in sync in the
same PR.
