# Onboarding a New Provider

Read this if you're lifting the baseimpl generator into a new Crossplane provider for the first time. After that, see the [README](../README.md#adding-a-new-resource) for per-resource work and [migration-checklist.md](migration-checklist.md) for migrating an existing legacy controller.

## Prerequisites

- A Go module for the provider (any module path).
- Crossplane runtime v2 (`crossplane-runtime/v2`).
- An existing ProviderConfig type and a way to construct your service client from it (your provider almost certainly already has both — there's no requirement they look any particular way).

## Step 1 — wire up the generator

Add a `make generate-baseimpl` target that runs the binary. Two flags carry your project's identity:

```bash
go run ./hack/generators/baseimpl \
  --module=github.com/your-org/your-provider \
  --provider-name=foo
```

Defaults exist (`github.com/sap/crossplane-provider-btp`, `btp`) but are wrong for any project except this one. `--module` flows into every emitted import path; `--provider-name` is the default value of the `+kubebuilder:resource:categories=` list when a base type omits `+codegen:categories=`.

A few other flags exist for non-default directory layouts (`--base-dir`, `--cluster-dir`, `--namespaced-dir`, `--logic-ctrl-dir`, `--cluster-ctrl-dir`, `--namespaced-ctrl-dir`). Run with `--help` to see them. Stick with defaults unless you have a strong reason.

## Step 2 — adopt the directory layout

```
apis/
├── base/<group>/<version>/        ← you write base type definitions here
├── cluster/<group>/<version>/     ← generated; do not edit
└── namespaced/<group>/<version>/  ← generated; do not edit

internal/controller/
├── logic/<group>/<resource>/      ← you write per-resource business logic here
├── cluster/<group>/<resource>/    ← generated controller; do not edit
└── namespaced/<group>/<resource>/ ← generated controller; do not edit
```

The `apis/v1alpha1/` package stays where it is — your provider's existing ProviderConfig and any non-baseimpl types live alongside the generator's output.

## Step 3 — write a project-shared helper (recommended)

The generator delegates everything provider-specific to per-resource logic packages. In practice every provider ends up writing the same Setup/Connect boilerplate per resource — constructing a service client, wiring a tracker, etc. Factoring that into one shared file at `internal/controller/logic/setup.go` keeps each per-resource package down to a handful of lines.

This helper is **not part of the generator's contract.** It exists purely as a project-local utility. Path, name, and contents are entirely your call. The shape this provider settled on:

```go
// internal/controller/logic/setup.go
package logic

type Options = internalopts.YourCrossplaneOptions
type MakeExternal func(ctx context.Context, mg resource.Managed, kube client.Client) (managed.ExternalClient, error)

func Setup(mgr ctrl.Manager, o Options, obj client.Object, kind string, gvk schema.GroupVersionKind, mk MakeExternal) error {
    // wraps your project's reconciler-setup helper (e.g. providerconfig.DefaultSetup)
}

func BuildClient(ctx context.Context, mg resource.Managed, kube client.Client) (*svc.Client, Tracker, error) {
    // builds your provider's service client + tracker for the given mg
}

func DeleteBlockedError() error { /* your project's "in use, can't delete" sentinel */ }
```

If you don't want a shared helper, skip this step and inline the same logic into each per-resource `Setup`/`Connect`. The generator doesn't care either way.

## Step 4 — write per-resource logic packages

For each resource, create `internal/controller/logic/<group>/<resource>/<resource>.go` following the contract in [Adding a new resource — Implement the logic layer](../README.md#2-implement-the-logic-layer). The contract is short: `Options`, `Setup`, `Connect`, `Observe`, `Create`, `Update`, `Delete`, `MigrateExternalName`. The generated controller calls these by name.

## Step 5 — wire up the generated `Setup`s

Each generated group has a `Setup` at `internal/controller/<scope>/<group>/zz_baseimpl_gen.setup.go`. Call those from your provider's top-level main.go (or wherever your existing controllers are registered), just like you'd register any other controller group.

## What you do NOT have to do

- Modify the generator binary, the templates, or anything in `hack/generators/baseimpl/`.
- Provide a particular options struct, tracker shape, or providerconfig package. Your project's existing types stay where they are; the per-resource logic packages bridge them to the generator's expected `(ctx, mg, kube)` interface.
- Match this provider's package paths (`btp/`, `internal/controller/providerconfig/`, etc.). None of those names appear in generated output.

## What you DO need at the project level

- A `Setup` helper that takes your options + a `MakeExternal` and registers a reconciler with the manager (your equivalent of `providerconfig.DefaultSetup`).
- A way to construct your service client from a managed resource's ProviderConfig (your equivalent of `providerconfig.CreateClient`).
- Whatever tracker / sentinel error semantics you want for `Observe`/`Delete`. The generated controller doesn't impose any — `logic.Observe` and `logic.Delete` are free to call or skip them.
