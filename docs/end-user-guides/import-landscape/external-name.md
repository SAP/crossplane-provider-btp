---
sidebar_position: 3
---

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

### CertBasedOIDCLogin

- Follows Standard: no - This resource does not support external name as it does not represent an external resource. Instead of using external name for importing, you can just create an instance of this resource.
- Format: Not applicable

### CloudFoundryEnvironment

- Follows Standard: yes
- Format: Environment Instance GUID (UUID format)
- How to find:

  - UI: BTP Cockpit → Subaccounts → [Select Subaccount] → Instances and Subscriptions → Instance ID
  - CLI: Use BTP ClI: `btp list accounts/environment-instance`

### Directory

- Follows Standard: yes
- Format: Directory GUID (UUID format)
- How to find:

  - UI: Global Account → Account Explorer → Directories → [Select Directory] → Directory ID
  - CLI: btp list accounts/directory (field: guid)

### DirectoryEntitlement

- Follows Standard: no (compound key, not a single GUID)
- Format:`<directory-id>/<service-name>/<plan-name>` (e.g. "abc-123-def-456/hana-cloud/hana")
- How to find:

  - UI: BTP Cockpit → Global Account → Account Explorer → [Select Directory] → Entitlements → Service Assignments > Service Technical Name and Plan
  - CLI: `btp list accounts/entitlement --directory <directory-id>` → `entitledServices[].name` and `entitledServices[].servicePlans[].name`

### GlobalaccountTrustConfiguration

- Follows Standard: no (origin key, not a GUID)
- Format: Origin key of the identity provider (e.g. "sap.custom")
- How to find:

  - UI: BTP Cockpit → Global Account → Security → Trust Configurations → [Origin column]
  - CLI: `btp list security/trust` → `Origin Key`

### KubeConfigGenerator

- Follows Standard: no - This resource does not support external name as it does not represent an external resource. Instead of using external name for importing, you can just create an instance of this resource.
- Format: Not applicable

### KymaEnvironment

- Follows Standard: yes
- Format: Environment Instance GUID (UUID format)
- How to find:

  - UI: BTP Cockpit → Subaccounts → [Select Subaccount] → Instances and Subscriptions → Instance ID
  - CLI: Use BTP ClI: `btp list accounts/environment-instance`

### KymaEnvironmentBinding

- Follows Standard: no - This resource does not support external-name based importing.
Instead of importing, create a new KymaEnvironmentBinding resource.
- Format: Not applicable

### KymaModule

- Follows Standard: yes
- Format: Kyma module name (e.g. "keda", "serverless")
- How to find:

  - UI: Kyma Dashboard → Modules → [Module Name]
  - CLI: `kubectl get kyma default -n kyma-system -o jsonpath='{.spec.modules[*].name}'`

### RoleCollection

- Follows Standard: no (uses name as identifier, not a GUID)
- Format: Role Collection Name (string)
- How to find:

  - UI: BTP Cockpit → Subaccount → Security → Role Collections → [Role Collection Name]
  - CLI: btp get security/role-collection `"<name>"` → `name`

### RoleCollectionAssignment

- Follows Standard: no - uses compound key as resource has no GUID available; user/group type derived from the mutually-exclusive spec fields userName/groupName
- Format: `<origin>/<userOrGroupName>/<roleCollectionName>` (e.g. "sap.default/jane.doe@example.com/Subaccount Administrator")
- Note: `spec.ForProvider` must match external name; mismatches will prompt an error
- How to find:

  - UI (RoleCollections): BTP Cockpit → Subaccount → Security → Role Collections
  - UI (User Assignments): BTP Cockpit → Subaccount → Security → Users → [Select entry] → Role Collections
  - CLI (RoleCollections): `btp --format json list security/role-collection --subaccount <subaccount-id>` (field: `name`)
  - CLI (User Assignments): `btp --format json get security/role-collection <role-collection-name> --subaccount <subaccount-id> --show-user-assignments` (fields: `origin`, `username`)

### ServiceInstance

- Follows Standard: no
- Format: ServiceInstance GUID (UUID format)
- Note: spec.ForProvider.SubaccountRef, spec.ForProvider.SubaccountSelector, or spec.ForProvider.SubaccountID must be set for adoption to work
- How to find:

  - UI: Subaccount → Services → Instances → [Select Instance] → Instance ID
  - CLI: btp list services/instance --subaccount `<subaccount-guid>` (field: id)

### Subaccount

- Follows Standard: yes
- Format: Subaccount GUID (UUID format)
- How to find:

  - UI: Global Account → Account Explorer → Subaccounts → [Select Subaccount] → Subaccount ID
  - CLI: btp list accounts/subaccount (field: guid)

### SubaccountApiCredential

- Follows Standard: no (uses credential name as identifier, not a GUID)
- Format: Credential Name (string)
- How to find:

  - UI: BTP Cockpit → Subaccount → Security → OAuth Clients → [Client Name]
  - CLI: `btp list security/app --subaccount <subaccount-id>` → `name`

### SubaccountTrustConfiguration

- Follows Standard: no (compound key, not a single GUID)
- Format:`<subaccount-id>/<origin>` (e.g. "abc-123-def-456/sap.custom")
- How to find:

  - UI: BTP Cockpit → Subaccount → Security → Trust Configurations → [Origin column]
  - CLI: `btp list security/trust --subaccount <subaccount-id>` → `Origin Key`

### Subscription

- Follows Standard: yes
- Format: `<appName>/<planName>`
- How to find:

  - UI: BTP Cockpit → Subaccounts → [Select Subaccount] → Instances and Subscriptions → [Select Subscription] → Application Technical Name and Plan
  - CLI: `btp list accounts/subscription` fields `app name` and `plan name`
