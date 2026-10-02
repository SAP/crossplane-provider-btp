# Examples

Three runnable scenarios demonstrating different configurations of
`function-btp-importer`. Each renders the desired resource map showing
what Crossplane would produce after the function runs, including
`crossplane.io/external-name` annotations stamped from the mock BTP API.

## Prerequisites

- The [`crossplane` CLI](https://docs.crossplane.io/latest/cli/)
- [Docker](https://docs.docker.com/get-docker/) running — `crossplane render` runs both `function-go-templating` and the Crossplane engine image (`CROSSPLANE_VERSION` in the Makefile) in containers
- The function running locally with a mock BTP server in **one terminal**:

```shell
# Replace <name> with: auto, explicit, or filters
go run . --insecure --debug --mock-btp=example/<name>/mock.yaml
```

The `--mock-btp` flag starts an in-process fake BTP HTTP server on port 18888.
All API calls (OAuth, SM, Accounts, Provisioning) are served by this mock using
the resource mappings defined in `mock.yaml`.

## Running examples

In a **second terminal**, use `make render` with `EXAMPLE=<name>`:

```shell
# Baseline: automatic import for all supported resources
go run . --insecure --debug --mock-btp=example/auto/mock.yaml &
make render EXAMPLE=auto

# Opt-in: only resources annotated with the lookup annotation are imported
go run . --insecure --debug --mock-btp=example/explicit/mock.yaml &
make render EXAMPLE=explicit

# Filtered: include/exclude regex patterns narrow which resources are attempted
go run . --insecure --debug --mock-btp=example/filters/mock.yaml &
make render EXAMPLE=filters
```

Output is printed to stdout **and** saved to `example/<name>/output/rendered.yaml`.

Each example uses `function-go-templating` to populate the desired resource map
with composed resources, then `function-btp-importer` stamps
`crossplane.io/external-name` on resources that match in the mock BTP API.

Each example directory contains:

- `mock.yaml` — mock BTP resource name→UUID mappings (consumed by `--mock-btp`)
- `required.yaml` — fake CIS credentials secret pointing to `http://127.0.0.1:18888`
- `composition.yaml` — pipeline with `function-go-templating` + `function-btp-importer`

---

## Scenario 1 — `auto/`

**What it demonstrates:** `mode: auto` — the function attempts to import every
supported BTP resource in the desired map that does not already have
`crossplane.io/external-name` set. If the BTP API returns no match, the
resource is left unchanged and the provider creates it normally.

**Resources created:** `ServiceInstance`, `Subaccount`, `KymaEnvironment`

**Key config:**

```yaml
mode: auto
cisCredentials:
  namespaceFromField: "metadata.labels[crossplane.io/claim-namespace]"
  secretSuffix: "-btp-provider-config"
```

**Pass 1:** The function reads the claim namespace from the XR label, constructs
the CIS secret name (`<namespace>-btp-provider-config`), and declares a
Requirements selector for it. Desired resources pass through unchanged.

**Pass 2:** The function receives the CIS secret, queries the BTP API for each
resource, and sets `crossplane.io/external-name` on any that match exactly one
result.

---

## Scenario 2 — `explicit/`

**What it demonstrates:** `mode: explicit` — the function only attempts import
for composed resources that carry the opt-in annotation:

```yaml
annotations:
  import.btp.sap.crossplane.io/lookup: "true"
```

Resources without this annotation are skipped silently. Useful when you want
precise control over which resources in a large Composition are candidates for
import, preventing accidental adoption.

**Resources created:** Two `ServiceInstance` resources — one annotated, one not.
Only the annotated one receives `crossplane.io/external-name`.

**Key config:**

```yaml
mode: explicit
```

---

## Scenario 3 — `filters/`

**What it demonstrates:** `mode: auto` combined with `include` and `exclude`
regex patterns (RE2 syntax) applied to the **composition resource name** (the
key in the desired map).

**Resources created:** Three `ServiceInstance` resources — `prod-instance`,
`dev-instance`, `test-skip-instance`. Only `prod-instance` receives
`crossplane.io/external-name` (matches `prod-.*` include pattern, and
`test-skip-instance` matches `.*-skip$` exclude pattern).

**Key config:**

```yaml
mode: auto
include:
  - "prod-.*" # only resources whose name starts with "prod-"
exclude:
  - ".*-skip$" # skip resources whose name ends with "-skip"
```

**Evaluation order:**

1. If `include` is non-empty and the name does not match any pattern → skip
2. If the name matches any `exclude` pattern → skip
3. Otherwise → attempt import

---

## Pass detection

The function uses a **two-pass flow** via `rsp.Requirements.Resources`:

| Pass                 | `reqs["cis-secret"]`         | What happens                                                   |
| -------------------- | ---------------------------- | -------------------------------------------------------------- |
| 1                    | Key absent                   | Declare CIS secret requirement; pass desired through unchanged |
| 2 (secret not found) | Key present, empty slice     | Fatal — secret does not exist                                  |
| 2 (secret found)     | Key present, populated slice | Query BTP API and set external-name                            |
