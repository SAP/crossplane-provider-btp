# Workload identity authentication

This opt-in PoC replaces the provider user's email/password authentication with a
rotating projected ServiceAccount assertion. CIS instances and bindings remain
manually provisioned, and their service-client secrets remain. Legacy password
ProviderConfigs select their existing path when workloadIdentity is absent.

## Common setup

1. Configure IAS Corporate IdP trust with the cluster OIDC issuer/JWKS, restrict
   user admission to the intended workload, and map the assertion to an active
   dedicated IAS user. Establish BTP platform trust and assign required roles.
2. In the BTP IAS application, explicitly select the Corporate IdP in Trust
   Corporate Identity Providers and Save. A displayed row is not proof that its
   trust checkbox is selected. Keep trust-all disabled.
3. Mount a projected ServiceAccount JWT without subPath so kubelet rotation is
   visible. Audience must equal the IAS discovery issuer; request600seconds.
   Keep the Kubernetes API token separate. Never store/assert the JWT in a CR,
   environment variable, example, local file or log.
4. Manually install central CIS credentials for account APIs. Environment CRs
   require a separate local CIS binding JSON under __raw in their Cloud Management
   Secret. Preserve these credentials until dependent resources finish deletion.

```yaml
apiVersion: btp.sap.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: workload
spec:
  globalAccount: example-global-account-subdomain
  cliServerUrl: https://cli.btp.cloud.sap
  cisCredentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: manual-cis-central
      key: credentials
  workloadIdentity:
    tokenFile: /var/run/secrets/workload/token
    identityProvider: example-platform-origin
    userEmail: provider@example.com
```

Omit serviceAccountSecret in workload mode. Both Terraform setup paths supply
assertion+idp. For native service-client authorization, create CIS instances with
parameter grantType=clientCredentials and use the returned client_credentials
bindings. This mode does not preserve a user authorization/audit principal.

## Native user-preserving CIS exchange

Use ordinary manually created user_token CIS bindings, with no grantType override,
and configure all three additional workloadIdentity fields together:

```yaml
    iasUrl: https://example.accounts.ondemand.com
    iasClientId: example-consumer-client-id
    iasResource: urn:sap:identity:application:provider:name:btp-principal-propagation
```

In IAS create a dedicated confidential OIDC consumer. Enable JWT Bearer. Configure
JWT client authentication bound to the exact projected issuer/JWKS/subject/audience
and explicitly select the Corporate IdP for user authentication. Leave public
client flows disabled and generate no IAS client secret. Add a dependency named
as configured above on the BTP application's advertised principal-propagation
API (the console may label it All APIs). Verify the saved dependency and selected
user trust, rather than assuming typed dropdown text or displayed rows persisted.

The provider uses the projected JWT as both user and client assertion. It requests
IAS's JWT access token for the BTP dependency, then exchanges it at the ordinary
CIS binding's XSUAA token endpoint using its manual clientid/clientsecret. It
verifies IAS mail and native CIS user_name/origin against configured workload
identity. No password, platform master credential, or service-token fallback is
used. Changing binding grant_type metadata cannot change broker grants.

## Cloud Foundry

CloudFoundryEnvironment passes workload email+origin to native environment creation,
automatically bootstrapping the workload Org Manager. It then uses projected JWT
CF UAA login for initial manager additions and manager observation. Its manual
local CIS binding can use service-client or configured user-preserving exchange.
Managers remain immutable and Update keeps the existing no-op behavior. This
feature introduces no automatic CIS instance/binding ownership or renewal.

KymaEnvironment/KymaEnvironmentBinding connectors also resolve workload metadata
through the common helper and retain their manual local CIS Secret. CF evidence
does not prove Kyma or native Subscription lifecycle coverage. Native Subscription
currently has its own service-client token source; its authorization semantics
are a separate validation item.

## Token lifecycle and verification

Terraform sessions reuse issuer/subject identity across JWT rotation and recycle
at15minutes; HTTP/backend401 evicts them for reconciliation retry. CF sessions
rebuild from the mounted token after refresh/authentication failure or15minutes;
access/refresh tokens remain in memory. Native CIS user tokens reread the mounted
JWT on token expiry/15minute replacement; backend401 invalidates the cached token
for the next reconcile without replaying a potentially mutating native API call.
Server session policy, revocation and long-duration expiry are separate gates.

Tests cover existing password paths, both Terraform builders, manual CIS metadata,
real CF/JWT protocol, concurrent login, rotation/principal isolation, access-token
expiry, rejected/absent refresh tokens and401 invalidation. Live tests must record
exact source/image and independently verify backend create/update/delete and audit
identity. Restart proves fresh login; a steady process crossing the original JWT
expiry and15minute recycle is needed to prove replacement from a new projection.

ServiceInstance name-based plan resolution still needs Service Manager credentials;
a pre-resolved plan ID skips it. Existing SI/SB ownership/rotation/recovery behavior
and service-specific credential requirements remain. Full legacy live regression,
all-resource parity, interrupted-create recovery and same-principal allow/deny
audit comparison are separate from the demonstrated authentication PoC.
