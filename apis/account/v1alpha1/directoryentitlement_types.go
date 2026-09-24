package v1alpha1

// External-Name Configuration:
//   - Resource: DirectoryEntitlement
//   - Follows Standard: no (compound key, not a single GUID)
//   - Format:`<directory-id>/<service-name>/<plan-name>` (e.g. "abc-123-def-456/hana-cloud/hana")
//   - How to find:
//     - UI: BTP Cockpit → Global Account → Account Explorer → [Select Directory] → Entitlements → Service Assignments > Service Technical Name and Plan
//     - CLI: `btp list accounts/entitlement --directory <directory-id>` → `entitledServices[].name` and `entitledServices[].servicePlans[].name`
