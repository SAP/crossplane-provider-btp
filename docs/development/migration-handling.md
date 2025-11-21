# Migration Handling

## Context
Sometimes resource API specs, lookup parameters and requirements for a Managed Resource change *BUT* someone is always relying on the previous version already. Across providers we lacke a unified definiton and process to handle such changes. This ADR proposes a consistent process for deciding on and managing migrations. 

## Background
```
Dev Sync (04.11.2025):

From Crossplane standpoint this is okay
We do not use it usually modify forProvider in the Observe
Traditionally this is filled in via a Webhook
We consider this is a migration -> one time changes should not be done in Observe
=> This goes into a broader migration ADR direction, how do we generally handle such cases?

=> If name is empty we use metadata.name, we do not update the spec as of now.
```
# ADR: Migration Strategies for Crossplane Providers

## Context
Resource API specs, lookup parameters, and Managed Resource requirements change over time while existing resources rely on previous versions. A unified approach for handling migrations is needed.

## Problem
Migrations affect both new and existing resources when changes are not purely additive. One-time transformations are required to move resources from prior to posterior state.

## Migration Approaches

### 1. Webhooks
Mutate resource specs at admission time. Traditional approach for field transformations.
- **When to use**: Field restructuring, adding required fields
- **Pros**: Clean separation, runs before controller logic
- **Cons**: Additional infrastructure

### 2. LateInitialize in Observe()
Populate missing fields during observation phase using external resource state.
- **When to use**: Backwards compatibility, setting defaults from external state
- **Pros**: Simple implementation, no additional components
- **Cons**: You should in theory not modify `spec.forProvider` (anti-pattern)

### 3. Kubebuilder Validations
Prevent invalid field combinations or enforce immutability at API level.
- **When to use**: Preventing updates to immutable fields, format validation
- **Pros**: Client side fails, clear error messages, defaulting
- **Cons**: Limited to validation, cannot transform data

### 4. Controller Logic
Handle migration within controller reconcile loops.
- **When to use**: State-dependent migrations, complex business logic
- **Pros**: Full access to resource state and external APIs
- **Cons**: Complicates controller logic

## Decision Matrix

| Scenario | Approach | Rationale |
|----------|----------|-----------|
| Add required field | Webhook + LateInitialize | Webhook for new, LateInitialize for existing |
| Immutable field enforcement | Kubebuilder validation | Prevents invalid updates at API level |
| Field format changes | Webhook | Clean transformation at admission |
| Backwards compatibility | LateInitialize (status only) | Populate from external state without spec modification |
| Complex state migrations | Controller logic | Requires external API context |

## Guidelines

1. **Avoid modifying `spec.forProvider` in Observe()** - violates Crossplane patterns
2. **Use webhooks for one-time transformations** - cleaner than controller logic
3. **Prefer validation over transformation** when possible
4. **Document migration paths** clearly for users and developers
5. **Test both new and existing resource scenarios**

## References
- [Crossplane Managed Resource Design](https://github.com/crossplane/crossplane/blob/main/design/one-pager-managed-resource-api-design.md)
- [Crossplane Import Guide](https://docs.crossplane.io/latest/guides/import-existing-resources/)

### Definitions

Migrations are a concern when changing anything related to the Kubernetes resource spec or logic within the controller.
Based on this definition, common assumptions are:
1. Resources have a state prior and posterior to their migration. [1]
2. As long as a feature is not purely additive, it affects both new and existing resources. [2]
3. There is a clear one time process which when applied to the prior state of a resource leading to its posterior state. 

### Default Behavior in Crossplane
- By default, the hubs and spokes model saves CRDs in a hub version
- This hub version is converted via a webhook to others during runtime if the CRD has not changed
- Resource versions are depicted by the `apiVersion` usually either `v1alpha` etc.

### Importing Existing External Resources (Official Flow) [3]
1. Create an MR manifest with `spec.managementPolicies: Observe`.
2. Add the external-name annotation with the external resource identifier. If not globally unique, also supply distinguishing `spec.forProvider` fields.
3. Apply the MR. Controller populates `status.atProvider`.
4. Copy the fields from `status.atProvider` to `spec.forProvider` and change `managementPolicies` to `*` to gain full control.

## Our Approach

### Definition

For the crossplane-provider-btp, we adopt a **layered migration strategy** that combines multiple approaches based on the type and complexity of the migration required. Our approach prioritizes backwards compatibility while ensuring clean migration paths for evolving BTP resource requirements.

#### Core Migration Principles

1. **Migration Type Classification**: Every change is classified into one of four categories to determine the appropriate migration approach:
   - **Additive Changes**: New optional fields or features
   - **Breaking Changes**: Required field additions, field removals, or format changes
   - **Immutable Field Changes**: Modifications to fields that cannot be updated post-creation
   - **Complex State Migrations**: Changes requiring external API context or multi-step transformations

2. **Backwards Compatibility First**: Existing managed resources must continue to function without user intervention whenever possible. Migration logic should handle legacy resource states gracefully.

3. **Explicit Migration Boundaries**: Clear documentation and validation messages guide users through required migration steps. No silent data transformations that could lead to unexpected behavior.

4. **Fail-Fast Validation**: Invalid configurations or migration states are caught early through API-level validations rather than runtime errors.
