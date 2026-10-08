# Request diagnostics

`--debug` records metadata-only CLI requests and reconciliation stages. CLI
response bodies are not drained for successful-request logging. Failed CLI
responses retain the existing bounded error-body logging. Session IDs are
represented by a short SHA-256 fingerprint, never their authentication value.

Each native reconcile carries `resource` (the parent Kind/name) and
`reconcileID`. Nested ServiceManager/CloudManagement instance and binding
operations inherit them. Stage records include elapsed `durationMs`, signed
`remainingMs` (`-1` if no deadline), and `contextError`:

- `connect`, `observe`, `create`, `update`, and `delete`;
- `observe-resources`, `instance-observe`, `binding-observe`,
  `instance-observe-repeat`, and `status-write` for SM/CM.

Failures are logged at Info; successful operations at Debug. When observation
and the following status write both fail, both errors are recorded without
changing the reconciler's returned error or resource state.

The pinned dependency overlay records `btp request queued`, session-lock
`waitMs` and `holdMs`, and whole-request duration. HTTP records add attempt
number, request action, request correlation ID, connection acquisition/TLS/time
to first byte, connection reuse, and response-body completion. Attempt counters
belong to logical request contexts, including retries, and are not stored in a
process-lifetime map. Async operations retain diagnostic values while keeping
Upjet's original independent cancellation and deadline.

## Build and test

Make's build/test/lint targets generate `.work/diagnostic-overlay/overlay.json`
against vendored dependencies. Source versions and SHA-256 hashes are checked;
dependency upgrades require reviewing and updating the hooks. The overlay does
not modify the module cache or tracked dependency code. Plain `go build` and
`go test` still work, but do not include the internal session/async hooks.

For direct diagnostic builds:

```sh
go mod vendor
go run ./hack/diagnostic-overlay .work/diagnostic-overlay
go build -overlay=.work/diagnostic-overlay/overlay.json ./cmd/provider
go test -race -tags=diagnostic -overlay=.work/diagnostic-overlay/overlay.json \
  ./pkg/diagnostics ./internal/clients/tfclient ./hack/diagnostic-overlay
```

The `diagnostic` test exercises the actual client's session mutex using a local
fake HTTP server: a holder blocks another request past its deadline, then the
trace must report the wait and expired context. No live BTP/Kubernetes access
is needed. The original lock, retry, and cancellation semantics are retained.

To diagnose a timeout, follow one `reconcileID`: identify which stage spends
the budget, then compare session wait against hold time. A high wait with short
holds indicates contention; a long hold can be explained by HTTP timing or
gaps between attempts. A failed `status-write` after negative `remainingMs`
is downstream of budget exhaustion, rather than evidence of a slow API write.
