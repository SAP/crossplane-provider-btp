---
sidebar_position: 3
---

# Exporter Clone Demo

This runbook exports one curated source subaccount, creates a new clone-ready
manifest, and provisions it through a locally running BTP provider in kind. It
is a demo workflow, not an automated teardown: it does **not** delete the kind
cluster, Kubernetes resources, provider process artifacts, or cloned BTP
resources.

## Prerequisites

Run from this repository with `direnv` already active. The local demo
environment supplies these variable names; do not put their values in command
lines, configuration files, or logs:

- `CIS_CENTRAL_BINDING`
- `BTP_TECHNICAL_USER`
- `BUILD_ID`
- `GLOBAL_ACCOUNT`
- `CLI_SERVER_URL`
- `SECOND_DIRECTORY_ADMIN_EMAIL`
- `TECHNICAL_USER_EMAIL`
- `IDP_URL`
- `BUILD_REGISTRY`

The script requires `CIS_CENTRAL_BINDING`, `BTP_TECHNICAL_USER`,
`GLOBAL_ACCOUNT`, `CLI_SERVER_URL`, and `TECHNICAL_USER_EMAIL`. It also requires
`BUILD_ID` unless a target subdomain is supplied explicitly. The other declared
variables are used to prepare the source landscape.

Before starting, ensure `go`, `btp`, `kind`, `kubectl`, and `jq` are on `PATH`.
A host container runtime is required by kind. Prepare one source subaccount and
use a selector that resolves to exactly that subaccount.

> [!WARNING]
> Curate the source subaccount carefully. It must contain only service instances
> whose unrecoverable creation parameters are not needed to recreate them. The
> BTP APIs do not return those parameters, so exporter output cannot restore
> them.

## Run the demo

```bash
scripts/exporter-clone-demo.sh '^source-subaccount$'
```

To choose the target rather than deriving it from the source subdomain and
`BUILD_ID`, pass a valid, unused subdomain:

```bash
scripts/exporter-clone-demo.sh '^source-subaccount$' \
  --target-subdomain source-subaccount-demo-123
```

All artifacts are written below a new `.work/exporter-clone-demo.*` directory.
The script prints its exact path at completion. It presents three prominent
operator phases:

1. **Exporting** builds the exporter, manifest transformer, and provider into
   `.work/`, previews the sanitized exporter command, then prints the complete
   generated exporter configuration. The configuration contains export-all,
   reference-resolution, and output settings; the source-subaccount selector is
   deliberately supplied as the displayed `--subaccount` command flag. After
   Enter confirmation, the script exports every supported resource from exactly
   one source subaccount and prints the complete preserved `raw-export.yaml`
   together with its path.
2. After export and twice after transformation, the script stops for an
   interactive confirmation. Review the raw manifest after export, then press
   Enter to begin **Transforming**. After transformation, press Enter to display
   its diff; then review that diff and press Enter again to begin
   **Provisioning**. There is deliberately no non-interactive bypass.
   Transforming prints each semantic transformation step with its reason,
   produces and validates `clone-ready.yaml`. That file begins with `---` and
   uses explicit `...` YAML document-end separators. The script does not print
   the full clone-ready YAML; after confirmation it presents a complete,
   colorized Git diff of raw export and clone-ready manifest, with both artifact
   paths.
3. **Provisioning** creates or reuses the named kind cluster, explicitly selects
   its context, applies BTP CRDs, creates provider secrets from inherited
   values, and applies the default `ProviderConfig`. Immediately before applying
   the clone-ready manifest, it previews the exact context-qualified `kubectl
   apply` command. It then presents a fixed, auto-refreshing (`watch`-like)
   view of color-coded `Ready` and `Synced` conditions for every managed
   resource until all are `True` or the timeout is reached.

The script reports each phase's completion. A non-zero error identifies the
active phase; provisioning failures also print per-resource conditions and
non-sensitive Kubernetes diagnostics.

The raw export is adoption-oriented: it retains source identity and uses
observe-only policies. The separate clone-ready manifest intentionally removes
that adoption state: it removes external-name annotations and stale source IDs
where generated references must resolve cloned resources, then sets managed
resources to `managementPolicies: ["*"]`. It changes the subaccount subdomain
and display name to the unique target subdomain, adds `TECHNICAL_USER_EMAIL` as
an administrator, and leaves Kubernetes resource names unchanged. Provisioning
shows Kubernetes `Ready` and `Synced` conditions only. These conditions are the
authoritative progress signal for the applied managed resources; the script does
not run extra BTP CLI lookups while it is provisioning.

## Inspect and retain the result

The completion output prints the raw export, clone-ready manifest, kubeconfig,
provider PID file, provider log, and ready-to-run `kubectl` inspection commands.
For example, use the printed kubeconfig and context to inspect resource state:

```bash
kubectl --kubeconfig .work/exporter-clone-demo.XXXXXX/kubeconfig \
  --context kind-crossplane-provider-btp-exporter-demo \
  get -f .work/exporter-clone-demo.XXXXXX/clone-ready.yaml -o wide
kubectl --kubeconfig .work/exporter-clone-demo.XXXXXX/kubeconfig \
  --context kind-crossplane-provider-btp-exporter-demo \
  get events --all-namespaces --sort-by=.lastTimestamp
```

On failure, the script prints non-sensitive Kubernetes status and events and
points to the provider log. It leaves all resources and artifacts available for
inspection. Stop the local provider with the `kill` command printed by the
script when finished. Because no teardown occurs, each later clone needs a new
`BUILD_ID` or a different target subdomain.

## Offline validation

`--help` requires no environment variables. `--dry-run` validates argument
shape and required environment-variable *names* only; it does not access BTP,
kind, Kubernetes, build artifacts, or the filesystem. It requires an explicit
target subdomain because deriving one requires the source export.

Use only synthetic placeholders for this check:

```bash
scripts/exporter-clone-demo.sh --help
env \
  CIS_CENTRAL_BINDING='{}' \
  BTP_TECHNICAL_USER='{"email":"demo@example.invalid","username":"demo","password":"placeholder"}' \
  GLOBAL_ACCOUNT=demo-global-account \
  CLI_SERVER_URL=https://example.invalid \
  TECHNICAL_USER_EMAIL=demo@example.invalid \
  scripts/exporter-clone-demo.sh '^demo-source$' \
    --target-subdomain demo-source-dry-run --dry-run
```

Do not run the real demo from an agent sandbox: kind needs a host container
runtime, and the live workflow connects to BTP using external credentials.

The repository's safe script test uses mocked tools and a pseudo-terminal to
exercise phase headings, all four required Enter confirmations, sanitized
command previews, exporter-configuration and raw-manifest display, colorized
complete raw-to-clone-ready Git diff, every transformation step and rationale, and
colorized auto-refreshing per-resource Kubernetes provisioning output. It uses
only synthetic values and does not contact BTP, Kubernetes, or a container
runtime:

```bash
bash scripts/exporter-clone-demo_test.sh
```
