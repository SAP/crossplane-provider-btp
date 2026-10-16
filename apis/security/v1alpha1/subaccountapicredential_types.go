package v1alpha1

// External-Name Configuration:
//   - Resource: SubaccountApiCredential
//   - Follows Standard: no (compound key; credentials are identified by subaccount ID and credential name)
//   - Format: `<subaccount-id>/<name>` (e.g. "abc-123-def-456/my-credential")
//   - Note: Existing name-only annotations are migrated automatically to the compound-key format; importing/adopting existing credentials is unsupported. The annotation identifies the credential but does not prove ownership. A new managed resource that names an existing credential is rejected before connection data is published; this applies to secret-based and certificate-based credentials. Secret-based credentials have a write-once client_secret that BTP cannot return.
//   - Recovery: Delete the rejected managed resource and choose a different credential name, or intentionally remove the existing BTP credential before creating a new managed resource. Rejection is terminal for that managed resource; do not clear its annotation or edit its spec to retry. This is not automatic rotation or lost-secret recovery.
//   - Upgrade: When upgrading the BTP provider from a version without lifecycle receipts to one that adds them, pre-existing resources without receipts retain their identity and connection Secret but are treated as ownership-uncertain; they are not automatically granted continued management or external deletion. Administrators must use the restricted, audited legacy ownership-resolution procedure; matching identity or Secret contents alone is insufficient evidence. Interrupted Creates with unresolved outcomes are also blocked pending resolution.
//   - How to find:
//     - UI: BTP Cockpit → Subaccount → Security → OAuth Clients → [Client Name]
//     - CLI: `btp list security/app --subaccount <subaccount-id>` → `name`
//
