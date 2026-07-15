package v1alpha1

// External-Name Configuration:
//   - Resource: SubaccountApiCredential
//   - Follows Standard: no (compound key; credentials are identified by subaccount ID and credential name)
//   - Format: `<subaccount-id>/<name>` (e.g. "abc-123-def-456/my-credential")
//   - Note: Existing name-only annotations are migrated automatically to the compound-key format; importing/adopting existing credentials is unsupported.
//   - How to find:
//     - UI: BTP Cockpit → Subaccount → Security → OAuth Clients → [Client Name]
//     - CLI: `btp list security/app --subaccount <subaccount-id>` → `name`
//
