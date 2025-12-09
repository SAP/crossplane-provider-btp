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

## Generated Data Below

### Directory

- Follow Standard: yes
- Format: Directory GUID (UUID format)
- How to find:

  - UI: Global Account → Account Explorer → Directories → [Select Directory] → Directory ID
  - CLI: btp list accounts/directory (field: guid)

### Subaccount

- Follow Standard: yes
- Format: Subaccount GUID (UUID format)
- How to find:

  - UI: Global Account → Account Explorer → Subaccounts → [Select Subaccount] → Subaccount ID
  - CLI: btp list accounts/subaccount (field: guid)
