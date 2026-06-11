# Behavior sketch — RoleCollectionAssignment ADR compliance

## Decisions made (review before implementing)

These are the conscious choices that shaped the sketch below. Anything here can be revisited; the rest of the sketch follows the ADR mechanically.

| #   | Decision                                    | Choice                                                                                                                   | Alternatives considered                                                                                                      | Rationale                                                                                                                                                                                                                                                                                                 |
| --- | ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | **Compound-key format**                     | Three-part: `origin/user-or-group/roleCollection`                                                                        | Four-part `type/origin/name/roleCollection` (self-describing); two-part dropping origin                                      | userName/groupName are mutually exclusive (XValidation, `rolecollectionassignment_types.go:13`) and immutable, so `isUserAssignment(cr)` recovers the type from spec without ambiguity. Four-part is the stricter ADR reading. **Needs review by more experienced colleagues.**                           |
| 2   | **Legacy migration UX**                     | Auto-migrate (CF env style): on `externalName == cr.Name`, look up by spec and rewrite the annotation                    | Subscription-style "guide the user with an error message"; drop legacy support entirely                                      | Compound key is fully spec-derivable, so the controller can do this deterministically with no risk of duplicate Creates. Best UX during the migration window. Caveat: GitOps tools that prune annotations (Argo `Replace=true`) defeat it — documented as a known limitation rather than handled in code. |
| 3   | **Drift reporting**                         | Strict ADR §Observe step 3: populate `ExternalObservation.Diff`, write `status.atProvider.Diff`, emit a Kubernetes event | Skip drift reporting (matches Subscription, Directory, RoleCollection, CFEnvironment — the majority of peer resources today) | User asked for strict ADR. Precedent exists in Kyma (`kymaenvironment.go:152-310`) and ServiceInstance (`serviceinstance.go:179-194`). Adds a `Diff` field to `RoleCollectionAssignmentObservation` and event-recorder wiring.                                                                            |
| 4   | **E2E import scope**                        | User-only flow                                                                                                           | Both user and group flows                                                                                                    | Faster to ship. Group code path covered by unit tests only.                                                                                                                                                                                                                                               |
| 5   | **Default-initializer suppression**         | Switch `zz_setup.go` to `DefaultSetupWithoutDefaultInitializer`                                                          | Keep current `DefaultSetup`                                                                                                  | Required prerequisite — without it, fresh resources reach Observe with `externalName == cr.Name` and the empty-annotation path of the ADR is unreachable. Matches the eight already-migrated resources.                                                                                                   |
| 6   | **Update behavior**                         | No-op success (`return managed.ExternalUpdate{}, nil`)                                                                   | Keep current `errNotImplemented`                                                                                             | All four spec fields are immutable via XValidation (`rolecollectionassignment_types.go:16-26`), so update can never have work to do; current error path would loop forever if Observe ever returned `ResourceUpToDate: false`.                                                                            |
| 7   | **Async deletion handling**                 | None                                                                                                                     | Track deletion state in status                                                                                               | `RemoveRoleCollection` is synchronous in XSUAA — the ADR's async-delete branch doesn't apply.                                                                                                                                                                                                             |
| 8   | **Delete on legacy sentinel (race)**        | Return parse error and rely on retry                                                                                     | Rebuild compound key from spec inside Delete to mirror the legacy Observe branch                                             | The race window (legacy match in Observe → MR deleted before `kube.Update` lands) is narrow and self-healing on the next reconcile. Rebuilding from spec inside Delete duplicates logic and silently masks the unexpected state. **Flag for review** — colleagues may prefer the belt-and-braces option.  |
| 9   | **Delete on empty / garbage external-name** | Treat both as parse error                                                                                                | Defensive success-on-empty path                                                                                              | Empty cannot arise through normal flow (Observe never returns `ResourceExists: true` for empty). If it occurs anyway, it indicates external mutation or a bug; silently succeeding would risk leaking the XSUAA assignment. A loud error surfaces the unexpected state.                                   |

## Setup (`zz_setup.go`)
Switch `DefaultSetup` → `DefaultSetupWithoutDefaultInitializer`. Fresh resources now reach Observe with `externalName == ""` instead of `externalName == cr.Name`.

## Helpers (new)
- `BuildExternalName(cr) string` → `fmt.Sprintf("%s/%s/%s", origin, IdentifierName(cr), roleCollection)`.
- `ParseExternalName(s) (origin, name, rc string, err error)` → `strings.Split(s, "/")`, length 3, all non-empty, no leading/trailing whitespace, total length ≤ 512.

## Observe
1. `externalName := meta.GetExternalName(cr)`.
2. **Empty** → `ResourceExists: false`. Create will run.
3. **Legacy sentinel `externalName == cr.Name`** → backwards-compat branch:
    - Build expected key from spec via `BuildExternalName(cr)`.
    - Call `client.HasRole(ctx, spec.origin, IdentifierName(cr), spec.roleCollection)`.
    - If true: `meta.SetExternalName(cr, expected)` + `kube.Update(cr)`, then fall through to the normal path with the spec values.
    - If false: `ResourceExists: false`. Create will run and write the same compound key.
4. **Non-empty, non-legacy** → `ParseExternalName(externalName)`:
    - Parse error → return error (ADR §Observe step 2).
    - Success → proceed with parsed `(origin, name, roleCollection)`.
5. `client.HasRole(ctx, parsedOrigin, parsedName, parsedRC)`:
    - Not found → `ResourceExists: false` (drift, not error).
    - Found → continue.
6. **Drift detection (strict ADR):**
    - Compare parsed `(origin, name, roleCollection)` against spec `(Origin, IdentifierName(cr), RoleCollectionName)`.
    - If different, build a diff string (`cmp.Diff`), populate:
        - `cr.Status.AtProvider.Diff = diff` (new field on `RoleCollectionAssignmentObservation`),
        - `event.Warning("DriftDetected", diff)` via the recorder,
        - `ExternalObservation.Diff = diff`.
    - Return `ResourceExists: true, ResourceUpToDate: true`. Spec fields are immutable, so the diff is informational only.
7. `cr.Status.SetConditions(xpv1.Available())`, return `ResourceExists: true, ResourceUpToDate: true`.

## Create
1. `cr.Status.SetConditions(xpv1.Creating())`.
2. `c.client.AssignRole(ctx, spec.Origin, IdentifierName(cr), spec.RoleCollectionName)`.
3. On error → wrap and return; **do not set external-name**.
4. On success → `meta.SetExternalName(cr, BuildExternalName(cr))`. XSUAA returns no ID, so synthesise the key from the spec values just used.
5. Return.

## Update
No-op success: `return managed.ExternalUpdate{}, nil`. All four spec fields are immutable via `XValidation` (`rolecollectionassignment_types.go:16-26`); the current `errNotImplemented` would loop forever if Observe ever returned `ResourceUpToDate: false` and must be removed.

## Delete
1. `cr.Status.SetConditions(xpv1.Deleting())`.
2. `externalName := meta.GetExternalName(cr)`. Branch on annotation state:
    - **Legacy sentinel `externalName == cr.Name`** → narrow race window between legacy Observe match and `kube.Update`. Return parse error; the next reconcile completes the migration and Delete will be called again with the compound key. (Alternative under review: rebuild the key from spec here. See decision #8.)
    - **Otherwise** → `ParseExternalName(externalName)`. On parse failure (including empty annotation), return error. Empty can only arise via external mutation or a bug; surfacing the error is preferable to silently leaking the XSUAA assignment.
3. `c.client.RevokeRole(ctx, parsedOrigin, parsedName, parsedRC)`.
4. If the error is a 404 from the underlying XSUAA call (assignment already removed externally), swallow and return success. Otherwise wrap and return.
5. No async deletion-state branch — `RemoveRoleCollection` is synchronous.

## Types (`rolecollectionassignment_types.go`)
- Add `Diff string` to `RoleCollectionAssignmentObservation`.
- Add the standard `External-Name Configuration:` doc comment block above the struct (Follows Standard: no — compound key; format: `origin/user-or-group/roleCollection`; UI + `btp` CLI navigation).

## Use cases

### Pre external-name addition (legacy / migration window)
These cover existing clusters that ran against the current controller before this ADR work. The cluster annotation either holds `cr.Name` (default-initializer ran before suppression) or is empty in Git but `cr.Name` in the cluster.

- **U1 — Legacy resource, assignment exists in XSUAA**: existing MR with `externalName == cr.Name`. Observe step 3 looks up by spec → `HasRole` true → annotation rewritten to `origin/user-or-group/roleCollection` via `kube.Update` → fall-through marks Available. No Create called. End state: annotation migrated, resource Ready.
- **U2 — Legacy resource, assignment missing in XSUAA** (e.g. someone revoked it manually after the MR was created): Observe step 3 → `HasRole` false → `ResourceExists: false` → Create runs → `AssignRole` succeeds → annotation set to compound key. End state: annotation migrated, assignment re-created.
- **U3 — Legacy resource that GitOps re-applies frequently with `Replace=true`**: cluster annotation may revert to absent on each sync. With suppression in place, Observe sees `externalName == ""` (case U4 below), not the legacy sentinel. Calls out a known limitation: tooling that prunes annotations defeats the migration. Document; do not try to handle in code.
- **U4 — Legacy resource where Git holds an explicit `externalName: <cr.Name>`**: someone committed back the default-initialized value at some point. Observe treats it as legacy sentinel exactly like U1/U2. After migration, the next Argo sync will see Git value `<cr.Name>` ≠ cluster value compound-key. Document: users should drop the annotation from Git or update it to the compound key after the migration reconciles once.
- **U5 — Legacy resource deleted mid-migration (race)**: user deletes the MR between Observe step 3's legacy-match `HasRole` call and the subsequent `kube.Update`. Delete sees `externalName == cr.Name`, returns parse error; next reconcile Observe completes the rewrite and Delete proceeds normally. Self-healing within one reconcile cycle.

### Post external-name addition (steady state)
These cover everything once the annotation holds a valid compound key — either freshly written by Create, just migrated by the legacy branch, or set by the user during import.

- **P1 — Fresh apply via GitOps, no annotation in manifest**: Observe step 2 → empty → `ResourceExists: false` → Create runs → `AssignRole` succeeds → annotation written to cluster. Argo sees a server-set annotation, leaves it alone unless `Replace=true`. End state: assignment created, annotation in cluster only.
- **P2 — Steady-state reconcile**: Observe step 4 → parse succeeds → `HasRole` true → spec matches parsed key → no Diff → Ready. Most common path.
- **P3 — Import of pre-existing XSUAA assignment**: user creates MR with `managementPolicies: [Observe]` and `crossplane.io/external-name: origin/foo@example.com/MyRole` matching an assignment that already exists. Observe step 4 → parse succeeds → `HasRole` true → spec matches → Ready. User then flips to `managementPolicies: ['*']` to take ownership.
- **P4 — Import with mismatched spec** (compound key valid but `spec.Origin` etc. don't match the parsed key): `HasRole` true on the parsed values, then drift detection fires → `Diff` populated, event emitted, `status.atProvider.Diff` written. `ResourceUpToDate: true` because spec is immutable, so user must delete and recreate the MR with the right spec. The diff makes that visible; without strict drift reporting it would be silent.
- **P5 — External revoke after creation**: someone removes the assignment in XSUAA outside Crossplane. Observe step 5 → `HasRole` false → `ResourceExists: false` → Create runs → `AssignRole` re-creates the assignment → Ready again. Self-healing; annotation already correct, no rewrite needed.
- **P6 — Invalid external-name typed by user**: user sets `crossplane.io/external-name: garbage`. Observe step 4 → parse fails → error returned. Resource stays in error condition; user must fix the annotation. Matches ADR §Observe step 2.
- **P7 — Delete with valid annotation**: parse succeeds, `RevokeRole` called, returns success. The designed Delete path.
- **P8 — Delete after external revoke**: parse succeeds → `RevokeRole` returns 404 → swallowed → success. crossplane-runtime removes the MR on the next reconcile.
- **P9 — Disaster recovery: cluster rebuilt from Git only**: MR re-applied with no annotation. Falls back to U-style flow but cleanly: Observe → empty → `ResourceExists: false` → Create runs → `AssignRole` returns "already exists" → ADR-compliant error loop, annotation stays empty, user notified to set the compound key in Git. Documents the GitOps-strict path.
- **P10 — Delete with garbage or externally-stripped annotation**: parse fails → error returned. Surfaces the unexpected state rather than silently leaking the XSUAA assignment. User must fix the annotation or remove the finalizer manually.

## Tests
- **Unit** (`rolecollectionassignment_test.go`): rewrite `TestObserve` — empty (false), legacy sentinel + found (auto-migrates and adopts), legacy sentinel + missing (false), valid key + found, valid key + 404, invalid format (errors), drift case (Diff populated). Rewrite `TestCreate` — success sets compound key, error leaves annotation empty. Replace `TestUpdate` with a no-op success. Extend `TestDelete` — valid key path, 404 swallowed, legacy sentinel parse error, garbage parse error, empty annotation parse error.
- **E2E import** (`test/e2e/rolecollectionassignment_test.go`, new): user-only flow via `ImportTester`. Pre-create the user assignment in XSUAA, apply MR with the compound external-name, assert Ready + correct status. Group path covered only by unit tests.
- **Upgrade** (`test/upgrade/rolecollectionassignment_external_name_upgrade_test.go`, new): baseCR with `externalName == cr.Name`, run controller, assert annotation transitions to compound key and resource stays Ready.
