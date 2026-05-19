package v1alpha1

// External-Name Configuration:
//   - Resource: SubaccountTrustConfiguration
//   - Follows Standard: no (uses origin key as identifier, not a GUID)
//   - Format: origin key (string, e.g. "sap.custom")
//   - How to find:
//     - UI: BTP Cockpit → Subaccount → Security → Trust Configurations → [Origin column]
//     - CLI: `btp list security/trust --subaccount <subaccount-id>` → `originKey`
//
