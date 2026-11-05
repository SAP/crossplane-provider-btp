# function-btp-importer

A Crossplane Composition Function that automatically imports existing SAP BTP resources by setting `crossplane.io/external-name` annotations on desired composed resources. This enables Crossplane to adopt pre-existing BTP resources (service instances, bindings, subaccounts, environments) rather than creating new ones.

## Installation

Install the function on your Crossplane control plane:

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Function
metadata:
  name: function-btp-importer
spec:
  package: <registry>/function-btp-importer:<version>
```

Build and push the package with `make image REGISTRY=<registry>`. If the registry is private, add `packagePullSecrets` with read access to it.

## How It Works

The function runs in a Crossplane composition pipeline. For each supported resource in the desired state, it queries the BTP APIs to find a matching existing resource and stamps its UUID as the external-name annotation. The Crossplane provider then adopts the resource instead of creating a duplicate. `Subscription` is the one kind that needs no lookup: its external-name is the `appName/planName` key the provider itself uses, so it is stamped straight from the spec and existence is left to the provider's own Observe.

### Supported Resources

| Kind                      | API Version                                         | Lookup Key                        |
| ------------------------- | --------------------------------------------------- | --------------------------------- |
| `ServiceInstance`         | `account.btp.sap.crossplane.io/v1alpha1`            | `spec.forProvider.name`           |
| `ServiceBinding`          | `account.btp.sap.crossplane.io/v1alpha1`            | `spec.forProvider.name`           |
| `Subaccount`              | `account.btp.sap.crossplane.io/v1alpha1`            | subdomain + region                |
| `Subscription`            | `account.btp.sap.crossplane.io/v1alpha1`            | appName + planName (spec-derived) |
| `ServiceManager`          | `account.btp.sap.crossplane.io/v1alpha1`, `v1beta1` | instance name + binding name      |
| `CloudManagement`         | `account.btp.sap.crossplane.io/v1alpha1`, `v1beta1` | instance name + binding name      |
| `KymaEnvironment`         | `environment.btp.sap.crossplane.io/v1alpha1`        | name + type                       |
| `CloudFoundryEnvironment` | `environment.btp.sap.crossplane.io/v1alpha1`        | name + type                       |

### Two-Pass Flow

The function uses Crossplane's Requirements mechanism and executes in two passes:

**Pass 1:** The function declares a Requirement for the CIS credentials Secret. Crossplane fetches it from the cluster.

**Pass 2:** With credentials available, the function builds API clients and resolves external names for each desired resource.

### Identity Verification

A name match alone is not enough to adopt a `ServiceInstance`: two different services can share an instance name. When the spec declares `offeringName` and/or `planName`, the importer resolves the matched instance's plan and adopts only if the declared identity agrees in full; a mismatch blocks the import and the provider will not create anything over it. A spec that declares neither field is adopted as a plain name match. `ServiceBinding` imports are additionally checked to belong to an instance this composition manages.

The outcome is reported on the composite **and the claim** as the `ImportsVerified` condition:

| Status    | Reason              | Meaning                                                                      |
| --------- | ------------------- | ---------------------------------------------------------------------------- |
| `True`    | `ResourcesImported` | N existing BTP resource(s) adopted this pass                                 |
| `True`    | `NothingMatched`    | Lookups ran, nothing matched; the provider will create                       |
| `True`    | `NothingToImport`   | No supported resource needed a lookup (steady state)                         |
| `Unknown` | `LookupErrors`      | N lookup(s) failed (transient); retried next reconcile                       |
| `False`   | `IdentityMismatch`  | A name matched but its offering/plan disagreed with the spec; import refused |

### Adopted Resources

Once a resource is adopted (its observed managed resource carries a real external-name), the importer skips the BTP lookup on every later reconcile but still writes that external-name into the render. Skipping the annotation would strip it from the managed resource and start a re-import loop. Resources are only looked up while they are still unbound, so a steady-state composition makes no BTP calls at all.

Rotation-managed `ServiceBinding`s must be kept **out** of the import population (unannotated in `explicit` mode, or excluded via `exclude`): lookups cannot match their suffixed names, and mirroring a rotating external-name can revert the provider's rotation write.

## Configuration

The function is configured via its `Input` in the composition pipeline step:

```yaml
- step: import-btp
  functionRef:
    name: function-btp-importer
  input:
    apiVersion: import.btp.sap.crossplane.io/v1beta1
    kind: Input
    mode: auto
    secretRef:
      name:
        value: "my-btp-secret"
      namespace:
        value: "my-namespace"
      key: "cisCredentials"
```

### `secretRef` (required)

Tells the function where to find the CIS credentials Secret.

| Field       | Description                                                                         |
| ----------- | ----------------------------------------------------------------------------------- |
| `name`      | The Secret name. Exactly one of `value`, `fromFieldPath`, or `fromContextKey`.      |
| `namespace` | The Secret namespace. Exactly one of `value`, `fromFieldPath`, or `fromContextKey`. |
| `key`       | The data key within the Secret that holds the base64-encoded CIS credentials JSON.  |

Each of `name` and `namespace` accepts exactly one of three resolution strategies (setting more than one is rejected, both at runtime and by the CRD's validation rule):

```yaml
# Static value
name:
  value: "my-secret-name"

# Resolved from the observed XR at runtime
namespace:
  fromFieldPath: "metadata.labels[crossplane.io/claim-namespace]"

# Resolved from Crossplane pipeline context (set by a prior function)
name:
  fromContextKey: "btp.sap/cis-secret-name"
```

### `mode` (optional, default: `auto`)

| Value      | Behaviour                                                                                       |
| ---------- | ----------------------------------------------------------------------------------------------- |
| `auto`     | Attempts import for all supported resources without an external-name                            |
| `explicit` | Only attempts import for resources annotated with `import.btp.sap.crossplane.io/lookup: "true"` |

### `include` / `exclude` (optional)

RE2 regex patterns matched against the composition resource name (the key in the desired map).

```yaml
include:
  - "prod-.*" # Only attempt import for resources starting with "prod-"
exclude:
  - ".*-skip$" # Skip resources ending with "-skip"
```

When `include` is non-empty, only matching resources are considered. `exclude` is evaluated after `include`.

## Usage Examples

### Auto mode with static secret reference

```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: Composition
spec:
  mode: Pipeline
  pipeline:
    - step: create-resources
      functionRef:
        name: function-patch-and-transform
      input:
        # ... your resource templates
    - step: import-btp
      functionRef:
        name: function-btp-importer
      input:
        apiVersion: import.btp.sap.crossplane.io/v1beta1
        kind: Input
        mode: auto
        secretRef:
          name:
            value: "team-a-btp-provider-config"
          namespace:
            value: "team-a"
          key: "cisCredentials"
```

### Dynamic secret reference from pipeline context

A prior function in the pipeline builds the secret name/namespace from platform naming conventions and writes them to the Crossplane context:

```yaml
- step: import-btp
  functionRef:
    name: function-btp-importer
  input:
    apiVersion: import.btp.sap.crossplane.io/v1beta1
    kind: Input
    mode: auto
    secretRef:
      name:
        fromContextKey: "platform.sap/cis-secret-name"
      namespace:
        fromContextKey: "platform.sap/cis-secret-namespace"
      key: "cisCredentials"
```

### Explicit mode with selective import

```yaml
- step: import-btp
  functionRef:
    name: function-btp-importer
  input:
    apiVersion: import.btp.sap.crossplane.io/v1beta1
    kind: Input
    mode: explicit
    secretRef:
      name:
        value: "my-btp-secret"
      namespace:
        value: "my-namespace"
      key: "cisCredentials"
```

In explicit mode, only resources with this annotation are imported:

```yaml
metadata:
  annotations:
    import.btp.sap.crossplane.io/lookup: "true"
```

## Architecture

### Client Hierarchy

```text
CIS Client (always — from the CIS Secret)
└── SM Client (only when a resource still needs a lookup AND an observed resource supplies a subaccount ID)
    └── Provisioning Client (only when the SM client exists and environment resources are in desired)
```

Clients are assembled in `newResolver`. The SM admin binding is a per-subaccount singleton shared with provider-btp, so it is acquired lazily: a reconcile with nothing left to import never touches SM. If a client cannot be acquired (transient failure), a Warning is emitted and resources requiring that client are skipped — other resources proceed independently. `Subscription` and `Subaccount` lookups need no SM client.

Auth failures (401/403) from the SM binding are Fatal and stop the entire pipeline.

### Key Design Decisions

- **Never return a Go error from `RunFunction`** — all failures go through `response.Fatal` or `response.Warning`
- **Preserve pipeline state** — `response.To(req, ...)` copies prior function output; we only add annotations
- **Graceful degradation** — SM or provisioning client unavailable? Skip those resources, proceed with the rest
- **SM binding lifecycle** — temporary admin binding created on-demand, deleted via `defer sm.Close()`

## Development

### Prerequisites

- [Go](https://go.dev/doc/install), at the version in `go.mod`
- [Docker](https://docs.docker.com/get-docker/), for `make render` and `make image`
- The [`crossplane` CLI](https://docs.crossplane.io/latest/cli/), for `make render` and `make image`
- [golangci-lint](https://golangci-lint.run/), for `make lint`

### Make Targets

| Target          | Description                                                                              |
| --------------- | ---------------------------------------------------------------------------------------- |
| `make build`    | Compile the function                                                                     |
| `make test`     | Run unit tests                                                                           |
| `make generate` | Regenerate deepcopy and CRD manifests                                                    |
| `make lint`     | Run golangci-lint                                                                        |
| `make render`   | Render an example (`EXAMPLE=auto`) against the running function; see `example/README.md` |

### Local Development with Mock Server

Use `--mock-btp` to start an in-process mock BTP server (port 18888) for `crossplane render`. Each example ships its own mock data:

```bash
go run . --insecure --debug --mock-btp=example/auto/mock.yaml   # terminal 1
make render EXAMPLE=auto                                          # terminal 2 (needs Docker)
```

`make render` runs the Crossplane engine image pinned by `CROSSPLANE_VERSION` in the Makefile. Details and the other examples: `example/README.md`.

### Running Tests

```bash
make test          # Unit tests
go test -race ./...  # With race detector
go test -cover ./... # With coverage
```
