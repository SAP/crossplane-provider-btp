# Per-Resource Migration Checklist

Use this checklist when migrating a single legacy controller to the cluster + namespaced split via the baseimpl generator. The generator handles most of the mechanical work; this document tracks what you still have to do manually.

For first-time provider onboarding (write the project-shared helper, configure the generator binary), see [Onboarding a new provider](onboarding.md).

## Goal

The migrated **cluster-scoped** resource must be functionally and surface-identical to the previous hand-written one:

- Generated CRD YAML for the cluster variant is byte-identical (modulo regen ordering) to the pre-migration CRD. Storage version, served versions, schema, validation, printer columns, categories, deprecation warnings — all preserved.
- Resource behaviour is unchanged: the same reconcile path, the same external-name handling, the same tracker / sentinel behaviour. Anything a user can observe at the API or controller level stays the same.
- Existing test files require **only import-path changes**. No call-site rewrites, no signature adjustments to assertion code, no fixture rewrites for the cluster variant. Logic-package tests may need to forward an extra `mg` argument and nil-guard the new `Client.tracker` field — that's the only allowed test edit.

The **namespaced** variant must be logically equivalent to the cluster variant, and therefore to the previous hand-written controller. Same reconcile path, same external-name handling, same observable behaviour — the only differences are the ones forced by the scope split: the resource is namespaced, its references are `*xpv1.NamespacedReference` instead of `*xpv1.Reference`, the resolved-reference target group is the `m.`-prefixed namespaced one, and the spec embeds `xpv2.ManagedResourceSpec` instead of `xpv1.ResourceSpec`. These things are handled by the generator itself though.

Tests aren't duplicated across scopes. Both variants share the same logic package, and that's where the behaviour lives — so existing logic-package tests cover the namespaced variant for free. The cluster and namespaced controllers themselves are generated and contain no behaviour worth testing in isolation.

If your migration produces a CRD diff on the cluster variant or forces a non-import test change, treat it as a regression and investigate before merging.

## Marker quick reference

Which marker to apply in which case:

| Case | Marker(s) | Where |
|---|---|---|
| Make a base type generate cluster + namespaced variants | `+codegen:generate:scoped`, `+kubebuilder:object:generate=false` | On `Base<Resource>` struct |
| Override the `+kubebuilder:resource:categories=` value (defaults to provider name) | `+codegen:categories=<comma-list>` | On `Base<Resource>` struct |
| Mark a generated CRD version as deprecated | `+codegen:deprecatedversion:warning="<msg>"` | On `Base<Resource>` struct |
| Reference another resource via a plain string field on `Base<R>Parameters` | Sidecar `<Resource>References` struct with `+codegen:references` on the struct, plus per-field `+codegen:reference:target=`, `+codegen:reference:group=`, `+codegen:reference:apiversion=`, `+codegen:reference:field=Spec.ForProvider.<Field>` (and optionally `refName=`, `selectorName=`, `extractor=`, `refDescription=`, `immutableRef=true`) | New `<Resource>References` struct alongside the base type |
| Reference another resource via a struct embedded in the spec that already carries angryjet markers (e.g. an `XSUAACredentialsReference`-style struct) | `+codegen:references` (no parameters) | On the embed field inside `Base<R>Spec` |
| Generate a typed accessor on the `Managed<Resource>` interface | `+codegen:method:field=<FieldPath>` | On a method declaration in the `Managed<Resource>` interface |

Full descriptions of each marker, including all parameters, live in [Marker reference](../README.md#marker-reference) in the README.

## 1. APIs

- [ ] `apis/base/<group>/<version>/<resource>_types.go` — pure spec/status base type with `+codegen:generate:scoped` and `+kubebuilder:object:generate=false` markers. No Kind registration. (See [Defining a base type](../README.md#defining-a-base-type).)
- [ ] If the version directory is new, also `apis/base/<group>/<version>/doc.go` with a `+groupName=` marker.
- [ ] References declared via either a `<Resource>References` sidecar struct or a `+codegen:references` marker on an embedded spec field. (See [Reference markers](../README.md#reference-markers).)
- [ ] `make generate-baseimpl` run; check the new files appear under `apis/{cluster,namespaced}/<group>/<version>/`.
- [ ] `make generate` run; deepcopy / resolver methods generated for both variants.
- [ ] **Cluster CRD is byte-identical to pre-migration.** Diff the regenerated cluster CRD YAML against the version on the base branch. Any discrepancy in schema, printer columns, validation rules, categories, or deprecation warnings is a migration bug — track it down before continuing. Common causes: missing `+codegen:categories=`, a re-ordered field in the base spec, an angryjet marker that didn't make it into the embedded-reference path.

## 2. Controllers

- [ ] `internal/controller/logic/<group>/<resource>/<resource>.go` written, exporting the contract in [Implement the logic layer](../README.md#2-implement-the-logic-layer): `Options`, `Setup`, `Connect`, `Observe`, `Create`, `Update`, `Delete`, `MigrateExternalName`.
- [ ] If migrating from a hand-written legacy controller: the legacy connector's custom Setup/Connect plumbing has moved into `logic.Connect`. Tracker calls (`SetConditions`, `DeleteShouldBeBlocked`) that lived in the legacy controller's `Observe`/`Delete` have moved into the equivalent `logic.X` functions.
- [ ] External-name compliance: if the resource was using a non-GUID external name in its legacy form, implement `MigrateExternalName` to upgrade in place. Otherwise it can return `nil`.

## 3. Scheme registration

- [ ] Confirm the per-group aggregator at `apis/<scope>/<group>/zz_baseimpl_gen.<group>.go` is wired into your provider's top-level scheme registration. If your project's main scheme file is generator-managed by another tool (e.g. upjet's `apis/zz_register.go`), be careful **not** to add the entries there — they will be clobbered. Use a separate hand-maintained file (commonly `apis/template.go` or similar).
- [ ] `internal/controller/<scope>/<group>/zz_baseimpl_gen.setup.go` `Setup` is called from your provider's controller-registration entry point.
- [ ] Provider boots without `no kind <X> is registered` panics for either scope.

## 4. CRDs and packaging

- [ ] CRDs regenerated under `package/crds/` (or wherever your project publishes them).
- [ ] Package metadata lists the new CRDs if it enumerates them rather than globbing.
- [ ] Storage version + served versions correct on both variants. Multi-version groups: the namespaced group is derived by inserting `m.` before `crossplane.io` (e.g. `<group>.example.crossplane.io` → `<group>.example.m.crossplane.io`).

## 5. Tests

- [ ] **Existing cluster-variant test files: only imports change.** The migration target is that legacy test files keep compiling and passing with their assertion code unchanged — only the import paths are rewritten to point at `apis/cluster/<group>/<version>` (and friends) instead of the legacy locations. If a test wants to construct a CR or call a controller method and the migration forces you to rewrite the body, that's a sign the migration broke surface compatibility — fix the migration, not the test.
- [ ] Logic-package tests pass against the new `(ctx, mg, kube)` `Connect` signature and the `mg` parameter on `Observe`/`Create`/`Update`/`Delete`. Existing tests can usually pass `nil` for `mg` if they don't exercise tracker behaviour; tracker calls in logic should nil-guard the `Client.tracker` field for this reason. (This is the one allowed test edit per the *Goal* section above.)
- [ ] Unit tests for the new `ToBase()` / `FromBase()` round-trip, if your project has appetite for that level. (The generator emits these; they're rarely the cause of bugs but pin behaviour.)
- [ ] e2e fixtures added under your project's e2e tree for both cluster and namespaced variants. Common pitfall: the namespaced manifest needs `namespace:` set; cluster manifests don't. Cluster e2e fixtures from the pre-migration era should run unchanged.
- [ ] e2e suite runs green against both scopes.

## 6. Docs / release notes

- [ ] Project-level migration tracker (if you have one) ticked.
- [ ] CHANGELOG / release notes updated with any user-visible changes (group renaming, new namespaced variant, scope-specific behaviour).
- [ ] Breaking-change notes if the resource's spec shape changed during migration.
