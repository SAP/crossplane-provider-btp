package v1alpha1

// External-Name Configuration:
//   - Resource: SubaccountApiCredential
//   - Follows Standard: no (compound key; credentials are identified by subaccount ID and credential name)
//   - Format: `<subaccount-id>/<name>` (e.g. "abc-123-def-456/my-credential")
//   - Note: Existing name-only annotations are migrated automatically to the compound-key format. Setting this annotation does not enable import: if the identified credential already exists for a new managed resource, reconciliation reports unsupported adoption and does not acquire it. An annotation targeting an absent credential may still be used for normal creation.
//   - How to find:
//     - UI: BTP Cockpit → Subaccount → Security → OAuth Clients → [Client Name]
//     - CLI: `btp list security/app --subaccount <subaccount-id>` → `name`
//
