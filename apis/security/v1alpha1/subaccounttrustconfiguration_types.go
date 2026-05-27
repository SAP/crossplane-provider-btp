package v1alpha1

// External-Name Configuration:
//   - Resource: SubaccountTrustConfiguration
//   - Follows Standard: no (compound key, not a single GUID)
//   - Format: <subaccount-id>,<origin> (e.g. "abc-123-def-456,sap.custom")
//   - Note: spec.forProvider.subaccountRef/subaccountSelector/subaccountId are populated
//           automatically after Create/Import and are NOT required for the import/adoption flow.
//   - How to find:
//     - UI: BTP Cockpit → Subaccount → Security → Trust Configurations → [Origin column]
//     - CLI: `btp list security/trust --subaccount <subaccount-id>` → `Origin Key`
//
