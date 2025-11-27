# ADR: Migration Semantics for Provider API Evolution

## Context
Provider resources evolve over time. New fields appear, lookup mechanisms change, and the semantics of existing fields may need revision. Existing managed resources already rely on previous definitions, so any modification to the API must account for them.

The diagram illustrates two fundamentally different migration regimes: remaining within a single API version and introducing a new API version. These regimes impose different constraints and yield different migration strategies.

## Core Observation
An API version defines a contract. As long as the version identifier remains fixed, the contract cannot be broken. Any change made inside that version must preserve compatibility for all existing resources. When a revision requires altering or removing fields, changing meaning, or breaking assumptions, the change must be expressed as a new API version.

This distinction is the foundation for the new migration strategy.

## Problem
Historically the provider treated migrations as a matter of one time transformations or ad hoc controller behavior. This approach does not scale. The problem is to define a consistent rule set that describes which changes are allowed within a version, which require a new version, and how controllers must behave under each regime.

## Rules for Changes Within a Single API Version
Remaining inside an API version forbids breaking changes. The only changes allowed are additive. Existing fields must remain valid and retain their meaning. A controller must accept old resource states and continue operating correctly without forcing user modification.

This implies five invariants.

1. Existing fields cannot be removed or change type.  
2. Existing semantics cannot change.  
3. The controller may only expand its behavior by accepting additional inputs or performing additional checks. 
4. Do not alter resource specs via `LateInitialize`.
5. Enforcing fields that are not yet enforced is done by Webhooks. 

Under this regime the spec can only grow. Every new field must be optional or defaulted. Any lookup change must be additive, often expressed as a new field such as external name. If an older field becomes obsolete, the controller must tolerate it indefinitely and give precedence to the new field when both are present.

No resource level migration is performed. Migration is not applied to existing objects. The controller simply interprets the old and new fields in a deterministic manner.

## Rules for Introducing a New API Version
A new API version creates a new contract. At this point all breaking changes are permissible. Fields may be renamed, removed, or retyped. Semantics may be revised. The controller for the new version is free to be written from scratch.

A new version therefore enables clean specification. Old behaviors are not carried into the new version. Migration is not performed by mutating existing objects. Instead users adopt the new version explicitly by updating `apiVersion`. The change is intentional and visible.

No in place transformation of resources across versions is attempted. Crossplane’s conversion mechanisms only normalize storage versions, not semantics. Changing `apiVersion` is the boundary where breaking changes are allowed.

## Implications for Migration Strategy
The term migration needs careful use. Within a single version there is no migration in the transformative sense. The controller evolves by widening its acceptance. Existing resources simply continue to work because the old contract remains valid.

Actual migration occurs only when a user chooses the new API version. At that point they adopt the new schema and controller behavior. Any breaking change is expressed through that version bump.

This provides a predictable rule set.

1. Additive changes stay within the current API version.  
2. Breaking changes force a new API version.  
3. Controllers for a version never rewrite user spec fields to perform migrations.  
4. Removal of obsolete fields occurs only in the next API version.

## Interaction with Webhooks, LateInitialization, and Validation
These mechanisms are now tools within strict version boundaries.

Webhooks perform defaulting or validation for a single version but do not migrate resources to another shape.  
LateInitialize remains an internal convenience but does not modify user supplied spec fields.  
Validation enforces invariants inside the version but does not express cross version transitions.

Their scope is limited to maintaining internal consistency of a single version, not performing transformations across versions.

## Conclusion
API versioning is the only mechanism that enables breaking change. Remaining in the same version requires pure monotonic extension of the spec and controller. This separation simplifies reasoning, avoids ad hoc migrations, and ensures that users choose when to adopt changes rather than having them applied implicitly.
