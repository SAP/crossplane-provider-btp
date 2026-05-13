# Base Implementation Generator

## Overview

The **baseimpl generator** is a code generation tool that produces scope-specific Kubernetes API types and controllers from a single set of base type definitions. It enables you to define your resource schemas once in `apis/base/v1alpha1/` and automatically generates both **cluster-scoped** and **namespaced** variants with complete Crossplane controller implementations.

This eliminates the need to manually maintain duplicated code across scopes and ensures that business logic remains in a single, testable location.

### Key Benefits

- **Single source of truth** for resource field definitions
- **Two scopes from one definition** — cluster-scoped (`aicore.crossplane.io`) and namespaced (`aicore.m.crossplane.io`)
- **Generated controllers** that delegate to shared logic functions
- **Cross-resource references** with automatic scope-aware resolution
- **Type-safe conversions** between scoped types and base types via `ToBase()`/`FromBase()`

## Quick Start

### Running the Generator

```bash
make generate-baseimpl
```

After running the generator, you can run:

```bash
make generate
```

This will run the kubebuilder code generators to produce deepcopy methods and CRD YAMLs based on the generated API types.

---

## Defining Base Types

### Resource Type Definition

Define your base types in `apis/base/v1alpha1/` with the `+codegen:generate:scoped` marker:

Make sure to add the `+kubebuilder:skip` and `+kubebuilder:object:generate=false` markers to prevent CRD and deepcopy generation for the base types, as they are not meant to be used directly.

```go
// BaseConfigurationParameters are the configurable fields of a Configuration.
type BaseConfigurationParameters struct {
    // Name of the configuration
    // +kubebuilder:validation:Required
    Name string `json:"name"`

    // ScenarioID is the ID of the scenario
    // +kubebuilder:validation:Required
    ScenarioID string `json:"scenarioId"`
}

// BaseConfigurationObservation are the observable fields of a Configuration.
type BaseConfigurationObservation struct {
    // ID is the configuration ID assigned by AI Core
    ID string `json:"id,omitempty"`
}

// BaseConfigurationSpec defines the desired state of a Configuration.
type BaseConfigurationSpec struct {
    ForProvider BaseConfigurationParameters `json:"forProvider"`
}

// BaseConfigurationStatus represents the observed state of a Configuration.
type BaseConfigurationStatus struct {
    AtProvider BaseConfigurationObservation `json:"atProvider,omitempty"`
}

// BaseConfiguration is the base resource definition.
// +codegen:generate:scoped
// +kubebuilder:skip
// +kubebuilder:object:generate=false
type BaseConfiguration struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              BaseConfigurationSpec   `json:"spec"`
    Status            BaseConfigurationStatus `json:"status,omitempty"`
}
```

### Naming Convention

The generator derives the resource name by stripping the `Base` prefix:

| Base Type Name | Generated Resource Name | Generated File |
|---|---|---|
| `BaseConfiguration` | `Configuration` | `zz_generated.configuration.go` |
| `BaseDeployment` | `Deployment` | `zz_generated.deployment.go` |

The following companion types are expected to exist:

| Convention | Example |
|---|---|
| `Base<Resource>Parameters` | `BaseConfigurationParameters` |
| `Base<Resource>Observation` | `BaseConfigurationObservation` |
| `Base<Resource>Spec` | `BaseConfigurationSpec` |
| `Base<Resource>Status` | `BaseConfigurationStatus` |

### Required Markers

| Marker | Where | Purpose |
|--------|-------|---------|
| `+codegen:generate:scoped` | On the `Base<Resource>` struct | Triggers generation of scoped variants |
| `+kubebuilder:skip` | On the `Base<Resource>` struct | Prevents CRD generation for the base type |
| `+kubebuilder:object:generate=false` | On the `Base<Resource>` struct | Prevents deepcopy generation for the base type |

---

## Cross-Resource References

References allow one resource to refer to another by Kubernetes name, which gets resolved to the external name (e.g., an AI Core configuration ID). The generator creates scope-appropriate reference fields.

### Defining References

Create a `<Resource>References` struct with the `+codegen:references` marker:

```go
// DeploymentReferences defines reference configuration for code generation.
// +codegen:references
// +kubebuilder:object:generate=false
type DeploymentReferences struct {
    // +codegen:reference:target=Configuration,field=Spec.ForProvider.Configuration
    ConfigurationRef bool
}
```

The `+codegen:reference` marker has two parameters:
- **`target=<Resource>`** — The resource type being referenced (e.g., `Configuration`)
- **`field=<FieldPath>`** — The field path on the *base spec* where the resolved value should be stored (e.g., `Spec.ForProvider.Configuration`)

### What Gets Generated

For each reference, the generator creates three fields in the scoped `Parameters` struct:

**Cluster-scoped** (using standard Crossplane v1 references):
```go
Configuration         string          `json:"configuration,omitempty"`
ConfigurationRef      *xpv1.Reference `json:"configurationRef,omitempty"`
ConfigurationSelector *xpv1.Selector  `json:"configurationSelector,omitempty"`
```

**Namespaced** (using Crossplane v2 namespaced references):
```go
Configuration         string                    `json:"configuration,omitempty"`
ConfigurationRef      *xpv1.NamespacedReference `json:"configurationRef,omitempty"`
ConfigurationSelector *xpv1.NamespacedSelector  `json:"configurationSelector,omitempty"`
```

### How Reference Resolution Works

The reference resolution flow has several moving parts:

1. **User specifies** a `configurationRef` (or `configurationSelector`) in their Deployment YAML
2. **Crossplane's angryjet** generates `ResolveReferences()` methods (in `zz_generated.resolvers.go`) that resolve the reference to the external name of the target resource
3. **The resolved value** is stored in the `Configuration` field (the plain string field)
4. **`ToBase()` copies** the resolved value into `BaseDeploymentSpec.Configuration`
5. **The logic layer** reads `cr.Spec.Configuration` to get the resolved AI Core configuration ID

### The `BaseSpec` Extra Field Pattern

For references, the base spec needs a non-serialized field to carry the resolved value:

```go
type BaseDeploymentSpec struct {
    ForProvider   BaseDeploymentParameters `json:"forProvider"`
    // Configuration is resolved from the reference — not serialized.
    Configuration string `json:"-"`
}
```

The `json:"-"` tag ensures this field is not part of the CRD schema. It exists solely as a transport mechanism between `ToBase()`/`FromBase()` and the logic layer.

---

## Adding a New Resource

Follow these steps to add a new resource type (e.g., `Artifact`):

### Step 1: Define Base Types

Create `apis/base/v1alpha1/artifact_types.go`:

```go
package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type BaseArtifactParameters struct {
    Name       string `json:"name"`
    ScenarioID string `json:"scenarioId"`
}

type BaseArtifactObservation struct {
    ID string `json:"id,omitempty"`
}

type BaseArtifactSpec struct {
    ForProvider BaseArtifactParameters `json:"forProvider"`
}

type BaseArtifactStatus struct {
    AtProvider BaseArtifactObservation `json:"atProvider,omitempty"`
}

// +codegen:generate:scoped
// +kubebuilder:skip
// +kubebuilder:object:generate=false
type BaseArtifact struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              BaseArtifactSpec   `json:"spec"`
    Status            BaseArtifactStatus `json:"status,omitempty"`
}
```

### Step 2: Create the API Client

Create `internal/client/artifact/artifact.go` implementing the external API calls (Get, Create, Update, Delete).

### Step 3: Create the Logic Layer

Create `internal/controller/logic/artifact/artifact.go` with these functions:

```go
package artifact

func Connect(data []byte, resourceGroup string) (client.Client, error) { ... }
func Observe(c client.Client, ctx context.Context, cr *base.BaseArtifact) (managed.ExternalObservation, xpv1.Condition, error) { ... }
func Create(c client.Client, ctx context.Context, cr *base.BaseArtifact) (managed.ExternalCreation, xpv1.Condition, error) { ... }
func Update(c client.Client, ctx context.Context, cr *base.BaseArtifact) (managed.ExternalUpdate, error) { ... }
func Delete(c client.Client, ctx context.Context, cr *base.BaseArtifact) (managed.ExternalDelete, xpv1.Condition, error) { ... }
```

### Step 4: Run the Generator

```bash
make generate-baseimpl
```

### Step 5: Generate Deepcopy and CRDs

```bash
make generate
```

### Step 6: Add References (Optional)

If your resource references another resource, add a references struct in the same `_types.go` file:

```go
// ArtifactReferences defines reference configuration.
// +codegen:references
// +kubebuilder:object:generate=false
type ArtifactReferences struct {
    // +codegen:reference:target=Scenario,field=Spec.ForProvider.ScenarioID
    ScenarioRef bool
}
```

And add the corresponding transport field to the base spec:

```go
type BaseArtifactSpec struct {
    ForProvider BaseArtifactParameters `json:"forProvider"`
    ScenarioID  string                `json:"-"` // resolved from reference
}
```

---

## Generated Files Reference

For each base type, the generator creates:

### API Files (per scope)

| File | Description |
|------|-------------|
| `apis/{scope}/v1alpha1/zz_generated.{resource}.go` | Scoped type with Crossplane embedding |
| `apis/{scope}/v1alpha1/zz_generated.baseimpl.go` | `ToBase()`/`FromBase()` conversion methods |
| `apis/{scope}/v1alpha1/zz_generated.doc.go` | Package documentation |
| `apis/{scope}/v1alpha1/zz_generated.groupversion_info.go` | API group registration |
| `apis/{scope}/v1alpha1/zz_generated.providerconfig_types.go` | ProviderConfig type |
| `apis/{scope}/v1alpha1/zz_generated.providerconfigusage_types.go` | ProviderConfigUsage type |
| `apis/{scope}/zz_generated.aicore.go` | Top-level package with scheme registration |

### Controller Files (per scope)

| File | Description |
|------|-------------|
| `internal/controller/{scope}/{resource}/zz_generated.{resource}.go` | Full controller implementation |
| `internal/controller/{scope}/zz_generated.setup.go` | Controller registration for all resources |
| `internal/controller/{scope}/utils/zz_generated.utils.go` | ProviderConfig usage tracker utilities |
| `internal/controller/{scope}/providerconfig/zz_generated.providerconfig.go` | ProviderConfig controller |

### Files You Must NOT Edit

All `zz_generated.*` files are overwritten on every generator run. Never manually edit them.

### Files You Must Maintain

| File | Description |
|------|-------------|
| `apis/base/v1alpha1/*_types.go` | Base type definitions with markers |
| `internal/controller/logic/{resource}/*.go` | Shared business logic |
| `internal/client/{resource}/*.go` | External API client implementations |

---

## Scope Differences

| Feature | Cluster (`aicore.crossplane.io`) | Namespaced (`aicore.m.crossplane.io`) |
|---------|----------------------------------|---------------------------------------|
| Spec embedding | `xpv1.ResourceSpec` | `xpv2.ManagedResourceSpec` |
| Status embedding | `xpv1.ResourceStatus` | `xpv1.ResourceStatus` |
| Reference type | `*xpv1.Reference` | `*xpv1.NamespacedReference` |
| Selector type | `*xpv1.Selector` | `*xpv1.NamespacedSelector` |
| Connector | `WithExternalConnector` | `WithTypedExternalConnector` |
| PC usage tracking | `LegacyProviderConfigUsageTracker` | Not needed (built into v2) |
| ProviderConfig lookup | By name only | By name + namespace |
| Kubebuilder scope | `scope=Cluster` | `scope=Namespaced` |

---

## Logic Layer Contract

The generator expects the logic layer (`internal/controller/logic/{resource}/`) to export these exact function signatures:

```go
// Connect creates an API client from credentials.
func Connect(data []byte, resourceGroup string) (<client-type>, error)

// Observe checks the external resource state.
func Observe(client <client-type>, ctx context.Context, cr *base.Base<Resource>) (managed.ExternalObservation, xpv1.Condition, error)

// Create creates the external resource.
func Create(client <client-type>, ctx context.Context, cr *base.Base<Resource>) (managed.ExternalCreation, xpv1.Condition, error)

// Update updates the external resource.
func Update(client <client-type>, ctx context.Context, cr *base.Base<Resource>) (managed.ExternalUpdate, error)

// Delete deletes the external resource.
func Delete(client <client-type>, ctx context.Context, cr *base.Base<Resource>) (managed.ExternalDelete, xpv1.Condition, error)
```

**Important**: The logic layer works exclusively with `*base.Base<Resource>` types. It never imports or references the scoped types. This is what enables the same logic to work for both cluster and namespaced scopes.

### Setting the External Name

In your `Create` function, set the external name on the base resource:

```go
func Create(client Client, ctx context.Context, cr *base.BaseConfiguration) (managed.ExternalCreation, xpv1.Condition, error) {
    resp, err := client.Create(ctx, req)
    if err != nil {
        return managed.ExternalCreation{}, xpv1.Condition{}, err
    }

    // Set external name — this is critical for reference resolution
    meta.SetExternalName(cr, resp.Id)

    return managed.ExternalCreation{}, xpv1.Creating(), nil
}
```

The generated controller then copies annotations back: `cr.SetAnnotations(baseCR.GetAnnotations())`.

---

## Common Issues and Troubleshooting

### "Reference resolves to empty string"

**Cause**: The referenced resource hasn't been created yet, or hasn't had its external name set.

**Expected behavior**: This is normal when resources are applied simultaneously. Crossplane will retry until the reference resolves. The Deployment will remain in a "pending" state until the Configuration has an external name.

### "Generator output doesn't include my new type"

**Checklist**:
1. Does your base type have the `+codegen:generate:scoped` marker?
2. Does the type name start with `Base` (e.g., `BaseArtifact`)?
3. Is the marker directly above the struct (in the doc comment block)?
4. Did you run `make generate-baseimpl`?

### "Compilation error: missing ToBase/FromBase method"

**Cause**: The `zz_generated.baseimpl.go` file is out of date.

**Fix**: Re-run `make generate-baseimpl`.

### "CRD validation error: unknown field"

**Cause**: CRDs were not regenerated after adding new fields.

**Fix**: Regenerate CRDs after modifying base types:
```bash
make generate-baseimpl
make generate
```

### "Reference field not appearing in CRD"

**Cause**: Missing or incorrect `+codegen:references` marker.

**Checklist**:
1. The references struct name must be `<Resource>References` (e.g., `DeploymentReferences`)
2. The struct must have the `+codegen:references` marker
3. Each field must have `+codegen:reference:target=X,field=Y` in its doc comment
4. The `field` path must point to a valid field on the base spec

### "deepcopy generation fails for base types"

**Cause**: Missing `+kubebuilder:object:generate=false` marker on base types.

**Fix**: Ensure all `Base<Resource>` structs have:
```go
// +kubebuilder:object:generate=false
```

### "angryjet doesn't generate resolvers"

**Cause**: The `+crossplane:generate:reference:type=<Target>` comment marker is missing from the generated scoped type.

**Fix**: This marker is included in the scoped type template. Re-run `make generate-baseimpl` and then `go generate ./apis/...`.

---

## FAQ

### Q: Can I add custom fields to the scoped types?

**A**: No. All `zz_generated.*` files are overwritten on every generator run. Add fields to the base types in `apis/base/v1alpha1/` instead, and they will appear in both scoped variants.

### Q: Can I have a resource with references to multiple other resources?

**A**: Yes. Add multiple fields to the references struct:

```go
// +codegen:references
// +kubebuilder:object:generate=false
type DeploymentReferences struct {
    // +codegen:reference:target=Configuration,field=Spec.ForProvider.Configuration
    ConfigurationRef bool

    // +codegen:reference:target=Scenario,field=Spec.ForProvider.ScenarioID
    ScenarioRef bool
}
```

Each will generate its own `Ref`/`Selector` field pair.

### Q: How do I test my logic layer?

**A**: The logic layer works with base types only, so you can test it without any Kubernetes dependencies:

```go
func TestCreate(t *testing.T) {
    cr := &base.BaseConfiguration{
        Spec: base.BaseConfigurationSpec{
            ForProvider: base.BaseConfigurationParameters{
                Name: "test",
            },
        },
    }
    mockClient := &MockClient{}
    result, condition, err := Create(mockClient, context.Background(), cr)
    // Assert...
}
```

### Q: How do I change the API group name?

**A**: The group names are defined as constants `GroupNameCluster` and `GroupNameNamespaced` in `constants.go`. Update those constants and re-run the generator.

### Q: Why are there two different ProviderConfig tracker approaches?

**A**: Cluster-scoped resources use the legacy Crossplane v1 `ProviderConfigUsageTracker` pattern, while namespaced resources use Crossplane v2's built-in tracking. This is handled automatically by the generator.
