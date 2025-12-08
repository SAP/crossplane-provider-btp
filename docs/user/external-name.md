# External name

`External name` in `Crossplane` is a key concept that maps `Crossplane` resources to their corresponding external resources in the managed infrastructure.

## What is External Name

The `External name` is an annotation (`crossplane.io/external-name`) that stores the identifier of the actual resource in the external system. It bridges the gap between:

- Crossplane resource name: The Kubernetes-style name in your cluster
- External resource ID: The actual identifier in the provider's API (e.g., BTP Subaccount ID)

In the BTP provider you can use the `External name` annotation to import existing recourses.

## BTP resources

To import existing BTP resources you need to add annotation with existing resource identifier

```yaml
...
metadata.annotations.crossplane.io/external-name: <resource_uniq_ID>
...
```

## Standard Format

```go
// <ResourceName> is a managed resource that represents <description> in the SAP Business Technology Platform.
//
// External-Name Configuration:
//   - Follow Standard: yes|no [explanation if no]
//   - Format: <identifier description>
//   - How to find:
//     - UI: <navigation path>
//     - CLI: <command> (field: <field_name>)
//
```

## Key Elements

1. **Follow Standard**: Indicates if the external-name follows the standard pattern
   - `yes` - Uses standard identifier (typically a single GUID/UUID)
   - `no` - Requires explanation (e.g., composite key, special format)

2. **Format**: Describes the identifier structure and type

3. **How to find**: Provides both UI and CLI methods to locate the identifier

## How to generate docs for the CRD external name

```bash
 make docs.generate-external-name
 ```

## Generated Data Below

### GlobalAccount

- Follow Standard: yes
- Format: Global Account GUID (UUID format)
- How to find:

  - UI: Global Account → Account Explorer → Global Account Details → Global Account ID
  - CLI: btp get accounts/global-account (field: guid)

### Subaccount

- Follow Standard: yes
- Format: Subaccount GUID (UUID format)
- How to find:

  - UI: Global Account → Account Explorer → Subaccounts → [Select Subaccount] → Subaccount ID
  - CLI: btp list accounts/subaccount (field: guid)
