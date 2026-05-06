package v1alpha1

// External-Name Configuration:
//   - Resource: SubaccountApiCredential
//   - Follows Standard: no (uses credential name as identifier, not a GUID)
//   - Format: Credential Name (string)
//   - How to find:
//     - UI: BTP Cockpit → Subaccount → Security → OAuth Clients → [Client Name]
//     - CLI: btp list security/app --subaccount <subaccount-id> → `name`
//
