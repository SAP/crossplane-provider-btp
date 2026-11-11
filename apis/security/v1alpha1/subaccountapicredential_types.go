package v1alpha1

// External-Name Configuration:
//   - Resource: SubaccountApiCredential
//   - Follows Standard: no (compound key; credentials are identified by subaccount ID and credential name)
//   - Format: `<subaccount-id>/<name>` (e.g. "abc-123-def-456/my-credential")
//   - Note: Existing name-only annotations are migrated automatically to the compound-key format. Existing credentials are adopted only when available Terraform state can produce complete connection data, including the client secret; BTP Read cannot recover an unavailable write-once secret.
//   - How to find:
//     - UI: BTP Cockpit → Subaccount → Security → OAuth Clients → [Client Name]
//     - CLI: `btp list security/app --subaccount <subaccount-id>` → `name`
//
