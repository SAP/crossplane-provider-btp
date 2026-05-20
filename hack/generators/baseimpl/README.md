# Base Implementation Generator

## Overview

The **baseimpl generator** at `hack/generators/baseimpl/` produces scope-specific Kubernetes API types and controllers from a single set of base type definitions. You define a resource schema once under `apis/base/<group>/<version>/` and the generator emits a **cluster-scoped** variant under `apis/cluster/<group>/<version>/` and a **namespaced** variant under `apis/namespaced/<group>/<version>/`, plus controllers that delegate to a shared logic package.

### Key benefits

- **Single source of truth.** Field definitions live in one place.
- **Two scopes from one definition** — cluster-scoped (`<group>.example.crossplane.io`) and namespaced (`<group>.example.m.crossplane.io`).
- **Generated controllers** that delegate to your logic functions.
- **Cross-resource references** with scope-aware resolution (cluster `*xpv1.Reference` vs. namespaced `*xpv1.NamespacedReference`).
- **Multi-version groups.** A single group can host base types at any number of versions side-by-side; both versions register at runtime.
- **Provider-agnostic.** The generated controllers and API types reference only Crossplane runtime types and the project's logic package — no provider-specific imports leak in. To use this generator in another Crossplane provider, write the per-resource logic packages following the contract in [Adding a new resource](#adding-a-new-resource); no changes to the templates or the generator itself are required.

### Invocation

```bash
make generate-baseimpl   # baseimpl generator
make generate            # controller-gen + angryjet (deepcopy, resolvers, CRD YAMLs)
```

Run `make generate-baseimpl` first, then `make generate`. The second pass picks up the newly emitted types.

---

## Onboarding a new provider

Lifting the generator into a new Crossplane provider is a one-time, mostly-mechanical exercise. See [docs/onboarding.md](docs/onboarding.md) for the step-by-step walkthrough (prerequisites, CLI flags, directory layout, the project-shared helper pattern, what you do and do *not* have to write).

For migrating an individual existing legacy controller into the generator's split layout, see [docs/migration-checklist.md](docs/migration-checklist.md).

---

## Directory layout

```
apis/
├── base/<group>/<version>/<resource>_types.go  ← source of truth (you write this)
│   apis/base/<group>/<version>/doc.go          ← +groupName= marker
├── cluster/<group>/<version>/                  ← generated
│   apis/cluster/<group>/zz_baseimpl_gen.<group>.go        ← per-group aggregator
└── namespaced/<group>/<version>/               ← generated
    apis/namespaced/<group>/zz_baseimpl_gen.<group>.go     ← per-group aggregator

internal/controller/
├── logic/<group>/<resource>/                   ← shared business logic (you write this)
├── cluster/<group>/<resource>/                 ← generated controller
│   internal/controller/cluster/<group>/zz_baseimpl_gen.setup.go    ← per-group setup
└── namespaced/<group>/<resource>/              ← generated controller
    internal/controller/namespaced/<group>/zz_baseimpl_gen.setup.go ← per-group setup
```

Controller directories are **version-agnostic** — the same `<resource>` directory holds the controller regardless of which version of the base type drove generation.

All generated files are prefixed `zz_baseimpl_gen.` and are overwritten on every generator run; never edit them by hand.

---

## Defining a base type

A base type lives in `apis/base/<group>/<version>/<resource>_types.go`. Each version directory needs its own `doc.go`:

```go
// apis/base/example/v1alpha1/doc.go
// +kubebuilder:object:generate=true
// +groupName=example.crossplane.io
package v1alpha1
```

The base type itself follows the convention `Base<Resource>` with companion `Base<Resource>{Parameters,Observation,Spec,Status}` types:

```go
package v1alpha1

import (
    xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BaseResourceParameters struct {
    DisplayName string `json:"displayName"`
}

type BaseResourceObservation struct {
    ID string `json:"id,omitempty"`
}

type BaseResourceSpec struct {
    ForProvider BaseResourceParameters `json:"forProvider"`
}

type BaseResourceStatus struct {
    xpv1.ConditionedStatus `json:",inline"`
    AtProvider             BaseResourceObservation `json:"atProvider,omitempty"`
}

// BaseResource is the base resource definition for Resource.
// +codegen:generate:scoped
// +kubebuilder:object:generate=false
type BaseResource struct {
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              BaseResourceSpec   `json:"spec"`
    Status            BaseResourceStatus `json:"status,omitempty"`
}
```

The generator strips the `Base` prefix to derive the resource name (`BaseResource` → `Resource`). Files are emitted as `zz_baseimpl_gen.<lower-resource>_types.go`.

---

## Marker reference

### Resource-level markers (on `Base<Resource>`)

| Marker | Required | Effect |
|---|---|---|
| `+codegen:generate:scoped` | yes | Triggers scoped-type generation. Without it the type is ignored. |
| `+kubebuilder:object:generate=false` | yes | Prevents controller-gen from producing deepcopy methods on the base type — only the scoped variants need them. |
| `+codegen:categories=<list>` | no | Sets the `categories=` value in the scoped CRD's `+kubebuilder:resource:` marker. Defaults to the configured provider name. |
| `+codegen:deprecatedversion:warning="<msg>"` | no | Emits `+kubebuilder:deprecatedversion:warning=` on the scoped CRD. |

Free-form doc lines on the `Base<Resource>` struct (anything that is not a `+marker` line and not the boilerplate `BaseX is the base resource definition for X.` line) are copied verbatim above the generated scoped type.

### Reference markers

There are two ways to declare cross-resource references. Both produce the same generated output; the choice depends on whether the field already has angryjet markers in the base package.

**1. Sidecar `<Resource>References` struct** — for references whose resolved value lands on a `BaseXxxParameters` field that has no markers of its own. Each field of the sidecar struct represents one reference:

| Marker (above a sidecar struct field) | Required | Purpose |
|---|---|---|
| `+codegen:reference:target=<Target>` | yes | The target resource type (bare name, e.g. `OtherResource`, or fully-qualified `github.com/.../v1alpha1.Foo`). |
| `+codegen:reference:group=<group>` | yes | API group of the target (used for tracker tags). |
| `+codegen:reference:apiversion=<version>` | yes | API version of the target. |
| `+codegen:reference:field=Spec.ForProvider.<Field>` | yes | Path to the value field on the base spec that receives the resolved external name. |
| `+codegen:reference:refName=<Name>` | no | Override for the `Ref` field name. Defaults to `<Field>Ref`. |
| `+codegen:reference:selectorName=<Name>` | no | Override for the `Selector` field name. Defaults to `<Field>Selector`. |
| `+codegen:reference:extractor=<expr>` | no | Fully-qualified extractor expression. Defaults to angryjet's `ExternalName()`. |
| `+codegen:reference:refDescription=<text>` | no | Free-form doc comment for the generated `Ref` field. Use `\n` for line breaks. |
| `+codegen:reference:immutableRef=true` | no | Emits an `XValidation` rule that prevents the ref name from changing once set. |

The sidecar struct must itself carry `+codegen:references` and `+kubebuilder:object:generate=false`. Its name must be `<Resource>References` (e.g. `ResourceReferences`).

```go
// ResourceReferences declares references for code generation.
// +codegen:references
// +kubebuilder:object:generate=false
type ResourceReferences struct {
    // +codegen:reference:target=OtherResource
    // +codegen:reference:group=example.crossplane.io
    // +codegen:reference:apiversion=v1alpha1
    // +codegen:reference:field=Spec.ForProvider.OtherResourceID
    // +codegen:reference:immutableRef=true
    OtherResourceRef bool
}
```

**2. `+codegen:references` on an embedded spec field** — for references whose value field already has angryjet `+crossplane:generate:reference:*` markers in the base package. The generator discovers everything from those markers and from the existing `reference-group:` / `reference-apiversion:` struct tags on the matching `Ref` pointer, so no further authored input is needed:

```go
type BaseResourceSpec struct {
    ForProvider BaseResourceParameters `json:"forProvider"`

    // +codegen:references
    SharedCredentialsReference `json:",inline"`
}
```

When this marker is on a field, the generator walks the embedded type, harvests every `+crossplane:generate:reference:type=` / `refFieldName=` / `selectorFieldName=` / `extractor=` marker, reads `reference-group:` / `reference-apiversion:` from the matching `Ref` field's struct tag, and emits a local copy of the embedded struct in each scoped package with the markers angryjet needs.

The marker in this position takes no parameters; the embedded type's own annotations supply everything.

### Custom-method markers

| Marker | Where | Purpose |
|---|---|---|
| `+codegen:method:field=<FieldPath>` | On a method declaration in a `Managed<Resource>` interface | Generates an accessor method that returns a value extracted from the given field path on the scoped type. The base resource definition exposes the method through the generated `Managed<Resource>` interface. |

This marker is supported by the parser but not currently used in this codebase.

---

## What gets generated

For each `Base<Resource>` in `apis/base/<group>/<version>/`:

| File | Path | Contents |
|---|---|---|
| Scoped type | `apis/<scope>/<group>/<version>/zz_baseimpl_gen.<resource>_types.go` | The `<Resource>` Go type, its Spec, Status, list type, kind metadata, and `init()` registering with the scheme. |
| Conversion | `apis/<scope>/<group>/<version>/zz_baseimpl_gen.baseimpl.go` | `ToBase()` / `FromBase()` between scoped and base types; also the `Managed<Resource>` interface implementation. |
| Type aliases | `apis/<scope>/<group>/<version>/zz_baseimpl_gen.types.go` | Re-exports of exported base structs and consts/vars (so consumers can import from the scoped package without touching `base/`). |
| Doc | `apis/<scope>/<group>/<version>/zz_baseimpl_gen.doc.go` | Trivial `package <version>` shim. |
| Group/version info | `apis/<scope>/<group>/<version>/zz_baseimpl_gen.groupversion_info.go` | `+groupName=`, `+versionName=`, and the `SchemeBuilder`. |
| Controller | `internal/controller/<scope>/<group>/<resource>/zz_baseimpl_gen.<resource>.go` | Full Crossplane controller that calls `logic.{Connect,Observe,Create,Update,Delete}`. |

### Per-group aggregators

These are emitted once per `(group, scope)`, covering every version of the group:

| File | Path | Contents |
|---|---|---|
| Scheme registration | `apis/<scope>/<group>/zz_baseimpl_gen.<group>.go` | `package apis` with `init()` that adds every version's `SchemeBuilder.AddToScheme` to `AddToSchemes`. |
| Setup | `internal/controller/<scope>/<group>/zz_baseimpl_gen.setup.go` | `Setup(mgr, opts)` calling each resource's `Setup` across all versions. |

---

## Multi-version groups

A group can have base types at any number of versions side-by-side. To add a new version (for example `v1beta1`) to an existing group:

1. Create `apis/base/<group>/v1beta1/doc.go` with the same `+groupName=` value used in `v1alpha1`.
2. Drop your `Base<Resource>` files into `apis/base/<group>/v1beta1/`.
3. Run `make generate-baseimpl`.

The aggregator at `apis/<scope>/<group>/zz_baseimpl_gen.<group>.go` will register both `v1alpha1` and `v1beta1` `SchemeBuilder`s, and the controller setup file will wire up resources from both versions.

**Collision rule.** Because controller directories are version-agnostic (`internal/controller/<scope>/<group>/<resource>/`), defining the same `Base<Resource>` in two version directories of the same group would silently overwrite the controller. The generator detects this and fails with an error pointing at both source paths.

---

## Adding a new resource

### 1. Define the base type

Create `apis/base/<group>/<version>/<resource>_types.go` following the conventions in *Defining a base type*. If the version directory is new, also create `doc.go`.

### 2. Implement the logic layer

Create `internal/controller/logic/<group>/<resource>/<resource>.go` with these exact signatures (the generated controller calls them by name):

```go
package <resource>

import (
    "context"

    "github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
    "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
    "k8s.io/apimachinery/pkg/runtime/schema"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"

    base "<module>/apis/base/<group>/<version>"
)

// Options is the per-Setup configuration. Aliasing to a project-shared type lets the
// generated Setup/group-setup signatures stay uniform across resources.
type Options = <project>.Options

// Client carries everything Observe/Create/Update/Delete need: the API client, kube,
// any tracker the project wants to apply on the managed resource, etc.
type Client struct { /* … */ }

// Setup wires up this resource's controller. Typically delegates to a project-shared
// helper that constructs the reconciler with the project's preferred providerconfig
// and tracker setup.
func Setup(mgr ctrl.Manager, o Options, obj client.Object, kind string, gvk schema.GroupVersionKind, mk MakeExternal) error

// Connect builds the per-call Client for one managed resource.
func Connect(ctx context.Context, mg resource.Managed, kube client.Client) (Client, error)

// CRUD methods. Each takes mg so the project's tracker (carried in Client) can act
// on the managed resource (SetConditions, DeleteShouldBeBlocked, …).
func Observe(c Client, ctx context.Context, mg resource.Managed, cr *base.Base<R>) (managed.ExternalObservation, error)
func Create(c Client, ctx context.Context, mg resource.Managed, cr *base.Base<R>)  (managed.ExternalCreation, error)
func Update(c Client, ctx context.Context, mg resource.Managed, cr *base.Base<R>)  (managed.ExternalUpdate, error)
func Delete(c Client, ctx context.Context, mg resource.Managed, cr *base.Base<R>)  (managed.ExternalDelete, error)

// MigrateExternalName runs at the start of Observe. Most resources can return nil.
func MigrateExternalName(c Client, ctx context.Context, mg resource.Managed, cr *base.Base<R>, kube client.Client) error

// MakeExternal is the constructor signature Setup forwards to the project helper.
type MakeExternal func(ctx context.Context, mg resource.Managed, kube client.Client) (managed.ExternalClient, error)
```

The logic package owns *all* provider-specific plumbing: API client construction, ProviderConfig resolution, reference trackers, deletion-block sentinels. The generated controller imports nothing project-internal beyond this logic package and the scoped API types. The same logic implementation drives both cluster and namespaced scopes because it only sees `*base.Base<Resource>` plus a generic `resource.Managed`.

In practice every project will have a small shared helper (this repo: `internal/controller/logic/setup.go`) that wraps providerconfig + tracker boilerplate so each per-resource `Setup`, `Connect`, etc. is one or two lines. That helper is not part of the generator's contract — projects choose the path and naming.

#### `mg` and `LegacyManaged`

Both cluster-scoped and namespaced CRs satisfy `resource.Managed` from `crossplane-runtime/v2/pkg/resource`, so the `mg` parameter works under either scope. Cluster-scoped CRs additionally satisfy `resource.LegacyManaged`; namespaced ones do not. Project helpers that need `LegacyManaged` (e.g. for the legacy ProviderConfig usage tracker) should type-switch:

```go
if legacy, ok := mg.(resource.LegacyManaged); ok {
    legacyTracker.Track(ctx, legacy)
}
```

### 3. Set the external name in `Create`

```go
meta.SetExternalName(cr, resp.Id)
```

The generated controller copies annotations back to the scoped CR with `cr.SetAnnotations(baseCR.GetAnnotations())`, which propagates the external name.

### 4. Run the generators

```bash
make generate-baseimpl
make generate
```

### 5. Add references (optional)

Pick the matching flavour from *Reference markers*. If your resource references a target through a plain string field on `BaseXxxParameters`, use the sidecar struct. If your resource embeds an existing reference-bearing struct (with angryjet markers already in place), put `+codegen:references` on the embed field and skip the sidecar.

---

## Scope differences

| Feature | Cluster | Namespaced |
|---|---|---|
| Group name | `<group>.example.crossplane.io` | `<group>.example.m.crossplane.io` |
| Spec embedding | `xpv1.ResourceSpec` | `xpv2.ManagedResourceSpec` |
| Status embedding | `xpv1.ResourceStatus` | `xpv1.ResourceStatus` |
| Reference type | `*xpv1.Reference` | `*xpv1.NamespacedReference` |
| Selector type | `*xpv1.Selector` | `*xpv1.NamespacedSelector` |
| `+kubebuilder:resource:scope` | `Cluster` | `Namespaced` |
| Resource group on resolved refs | `<group>.example.crossplane.io` | `<group>.example.m.crossplane.io` |

Resolved cross-resource references in the namespaced output point at namespaced versions of their targets — the generator rewrites the group automatically.

---

## Troubleshooting

**Generator output doesn't include my new type.** Check that the type starts with `Base`, that `+codegen:generate:scoped` is in its doc comment block, and that the version directory has a `doc.go` with `+groupName=`. Without that doc.go, the version is silently skipped.

**`group X: resource Y defined in multiple versions` error.** A `Base<Y>` exists in two version directories of the same group. Pick one. (Same resource name in two *different* groups is fine.)

**Reference resolves to empty string.** Normal during simultaneous apply; Crossplane retries until the target has an external name.

**`angryjet doesn't generate ResolveReferences` for a migrated cluster type.** angryjet doesn't follow imports across packages, so a base type that hides reference-bearing fields behind an embed (a struct in another package whose fields already carry `+crossplane:generate:reference:*` markers) needs the embed-marker variant of `+codegen:references` so the generator emits a local copy with the markers angryjet expects.

**CRD validation error: unknown field.** Re-run both `make generate-baseimpl` and `make generate`. The first regenerates the Go types; the second regenerates the CRD YAMLs and resolvers from those types.

**Compilation error: missing `ToBase`/`FromBase`.** `zz_baseimpl_gen.baseimpl.go` is out of date. Re-run `make generate-baseimpl`.

---

## FAQ

**Can I add fields to the generated scoped types?** No. Add them to the base type under `apis/base/<group>/<version>/`; both scoped variants pick them up automatically.

**Can I have a resource with multiple references?** Yes — either add multiple fields to the sidecar `<Resource>References` struct, or put `+codegen:references` on multiple embedded fields in the spec.

**How do I change the API group name?** Edit the `+groupName=` marker in the version directory's `doc.go`. The namespaced group name is derived automatically by inserting `m.` before `crossplane.io` (e.g. `example.crossplane.io` → `example.m.crossplane.io`).

**Can I test the logic layer without Kubernetes?** Yes. The logic layer takes `*base.Base<Resource>` directly; build one in-memory and call its functions with a mock client.
