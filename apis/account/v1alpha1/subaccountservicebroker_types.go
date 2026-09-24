package v1alpha1

// External-Name Configuration:
//   - Resource: SubaccountServiceBroker
//   - Follows Standard: no (compound key, not a single GUID)
//   - Format: `<subaccount-id>/<service-broker-id>` (e.g. "6aa64c2f-38c1-49a9-b2e8-cf9fea769b7f/6a55f158-41b5-4e63-aa77-84089fa0ab98")
//   - Note: import requires managementPolicies: ["*"]; observe-only import is not supported for this resource
//   - How to find:
//     - UI: Not available. The BTP cockpit does not show service brokers, only Service Marketplace and Instances and Subscriptions. Use the CLI or the Service Manager API.
//     - CLI: `btp list services/broker --subaccount <subaccount-id>` (field: id)
//
