# ResourceUsage Tracking — SAP/crossplane-provider-btp

**Date:** 2026-06-25
**Scope:** `internal/tracking/resourcetracker.go`, `apis/v1alpha1/resourceusage_types.go`, every controller calling `c.resourcetracker.Track()` in `Connect()`.
**Audience:** anyone touching `Connect()`/`Delete()` in this provider, anyone proposing changes to dependency-tracking.

---

## Context

BTP resources in this provider form a dependency graph. A `CloudManagement` (CIS instance) is provisioned through a `ServiceManager`. A `ServiceInstance` is provisioned through a `ServiceManager`. A `ServiceBinding` lives off a `ServiceInstance`. Entitlements, Kyma environments, CIS, and SM all live under a `Subaccount`. The k8s objects mirror that graph through `xpv1.Reference` fields tagged with `reference-group` / `reference-kind` / `reference-apiversion`.

**BTP-side cleanup is order-sensitive.** A `CloudManagement` whose backing `ServiceManager` has already disappeared cannot reach the BTP API to remove its CIS instance — the credentials it needs live in the SM-issued secret, which the SM controller cleaned up on its way out. The k8s finalizer on the `CloudManagement` then never clears, the MR stays in `Terminating` indefinitely, and the BTP instance leaks. Same story for `ServiceInstance` → `ServiceManager`, `ServiceBinding` → `ServiceInstance`, and downstreams of `Subaccount`.

Crossplane itself does not enforce delete ordering across MRs. Owner references between MRs would break Crossplane's "Crossplane owns its objects" model and would not survive composition revisions. So this provider runs its own dependency tracker on top of stock Crossplane: every reconcile, every controller stamps `ResourceUsage` CRs that record "downstream X depends on upstream Y", and each upstream's `Delete()` consults those records before allowing itself to be removed.

The mechanism rests on three invariants:

1. RU records exist and stay current — `Track()` runs on every downstream reconcile.
2. The upstream's `ResourceUsage` condition reflects whether anything still references it, so `DeleteShouldBeBlocked` can read it.
3. Nothing bypasses the block — no force-delete, no manual finalizer patch, no `deletionPolicy: Orphan` on an in-use upstream.

---

## Note: upstream Crossplane `Usage` came later

Crossplane shipped its own `Usage` kind in v1.14 (Oct 2023), graduated to beta in v1.19, now under `protection.crossplane.io/v1beta1` (the cluster also still exposes the older `apiextensions.crossplane.io/v1beta1.Usage` alias). It solves the same delete-ordering problem at the framework layer. Our tracker pre-dates it.

The two implementations differ in three load-bearing ways:

| | Our `ResourceUsage` | Upstream `Usage` |
|---|---|---|
| Created by | Provider stamps automatically via reflection on `xpv1.Reference` fields with `reference-*` struct tags | User or composition author declares one per dependency |
| Enforcement | Provider's `Delete()` returns an error after consulting `DeleteShouldBeBlocked` — advisory only | Crossplane-core validating admission webhook rejects DELETE at the API server |
| Replay on unblock | None — once an upstream is deletable, no automatic retry of the queued delete | `spec.replayDeletion: true` re-issues the original DELETE after the `Usage` is gone |

Tag-driven auto-stamping fits this provider's model — every cross-MR reference is already typed, no extra YAML. A future direction worth considering is generating `Usage` CRs from the same reflection pass that produces `ResourceUsage` today, inheriting webhook enforcement without touching controller code.

Source: [Crossplane Usage docs](https://docs.crossplane.io/latest/managed-resources/usages/).

---

## Mechanism

### 1. Reference discovery (`internal/tracking/resourcetracker.go:65`)

`Track(ctx, mg)` reflects over the MR using `mitchellh/reflectwalk`, finds every `xpv1.Reference` field carrying all three struct tags `reference-group`, `reference-kind`, `reference-apiversion` (`:154–175`), and emits one `ResolvedReference` per match. Fields without the tag trio are skipped. This is how a new field becomes "tracked" without writing controller code — just tag it.

Example (`apis/account/v1beta1/cloudmanagement_types.go:33`):

```go
ServiceManagerRef *xpv1.Reference `json:"serviceManagerRef,omitempty"
    reference-group:"account.btp.sap.crossplane.io"
    reference-kind:"ServiceManager"
    reference-apiversion:"v1alpha1"`
```

### 2. RU creation (`createTracking`, `:177–224`)

For each resolved reference:

1. `Get` the upstream by `(group, version, kind, name)`.
2. Build a `ResourceUsage` named `<sourceUID>.<targetUID>`.
3. Owner reference: the **target (downstream)**, with `BlockOwnerDeletion: false`. RU garbage collection follows the downstream — when the downstream is gone, its RUs are reaped by k8s.
4. Labels: `ref.orchestrate.cloud.sap/source-uid` and `…/target-uid` for indexed lookup.
5. `Apply` with `MustBeControllableBy(target.UID)` and `AllowUpdateIf(specsDiffer)` — idempotent and concurrency-safe between sibling reconciles of the same downstream.

### 3. Condition wiring (`SetConditions`, `:284–287`)

Each upstream MR calls `tracker.SetConditions(ctx, mg)` to refresh its `ResourceUsage` condition. `hasUsages` lists RUs by `LabelKeySourceUid == mg.UID` and writes:

- `True` + reason `ResourceUsagesFound` if any RU exists (`apis/v1alpha1/resourceusage_types.go:76`).
- `False` + reason `NoResourceUsagesFound` otherwise (`:77`).

### 4. Delete block (`DeleteShouldBeBlocked`, `:303–308`)

```go
func (r *DefaultReferenceResolverTracker) DeleteShouldBeBlocked(mg resource.Managed) bool {
    if hasIgnoreAnnotation(mg) { return false }
    return mg.GetCondition(v1alpha1.UseCondition).Reason == v1alpha1.InUseReason
}
```

- Keys on `Reason`, not `Status`. The block engages when reason is `ResourceUsagesFound`.
- The `ref.orchestrate.cloud.sap/ignore` annotation (`apis/v1alpha1/resourceusage_types.go:16`) opts out entirely — an escape hatch for higher-level orchestrators that need to override the check.

### 5. Enforcement surface

The block is implemented entirely by each upstream's `Delete()` returning an error after consulting `DeleteShouldBeBlocked`. There is no admission webhook in this repo; enforcement is cooperative within the provider's own reconcile loop. The contract is "Crossplane calls our `Delete()`, our `Delete()` refuses while RU records still exist."
