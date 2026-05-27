package v1alpha1

// External-Name Configuration:
//   - Resource: SubaccountTrustConfiguration
//   - Follows Standard: no (compound key, not a single GUID)
//   - Format: <subaccount-id>,<origin> (e.g. "abc-123-def-456,sap.custom")
//   - How to find:
//     - UI: BTP Cockpit → Subaccount → Security → Trust Configurations → [Origin column]
//     - CLI: `btp list security/trust --subaccount <subaccount-id>` → `Origin Key`
//
