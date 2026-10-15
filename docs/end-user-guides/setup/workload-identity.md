---
sidebar_position: 3
---

# Configure workload identity authentication

Use workload identity authentication to replace the BTP provider user's long-lived email/password
with a rotating Kubernetes ServiceAccount JWT. IAS maps the workload to a
passwordless user. The provider uses that identity for Terraform/CLI and Cloud
Foundry authentication. Native Cloud Management Service (CIS) calls can use either
an independent service-client token or a federated user token.

## Prerequisites and values to collect

- A provider version containing the `workloadIdentity` feature and its CRD fields.
  Earlier provider versions may not support these fields; verify the installed CRD.
- A Crossplane control plane whose provider ServiceAccount tokens have an OIDC
  issuer and public HTTPS discovery/JWKS endpoints reachable by IAS.
- A BTP global account and a **central CIS binding stored in a Kubernetes Secret**.
  Follow the existing [CIS setup](./configure-provider-btp.mdx#set-up-the-sap-cloud-management-service)
  and [binding Secret guide](./configure-provider-btp.mdx#create-a-secret-for-the-sap-cloud-management-service).
  `client_credentials` bindings are supported. Native environment APIs also need
  a local CIS binding in the target subaccount, exposed under `__raw` in their
  Cloud Management Secret. CF requires an available, entitled landscape.
- An IAS tenant eligible for BTP platform trust, with administrator access.
  An existing dedicated tenant can be reused. If another tenant is needed, use
  your organization's Identity Services onboarding process before proceeding;
  the provider does not create or activate IAS tenants.
- BTP administration rights for trust, role assignments, entitlements and manual
  service instance/binding creation.

Use this value map throughout the guide. Uppercase placeholders must be replaced
before applying manifests.

| Value | Meaning and source |
|---|---|
| `GA_SUBDOMAIN` | Global account subdomain from BTP account overview; not necessarily its UUID |
| `BTP_CLI_URL` | CLI server for the BTP landscape, e.g. `https://cli.btp.cloud.sap` |
| `IAS_ISSUER` | Exact `issuer` in IAS `/.well-known/openid-configuration` |
| `IAS_TOKEN_BASE_URL` | Host/base URL serving IAS `/oauth2/token`, without that suffix |
| `CLUSTER_ISSUER`, `CLUSTER_JWKS_URI` | Values returned by the cluster discovery document |
| `PROVIDER_NAMESPACE` | Namespace in which Crossplane runs the provider, e.g. `crossplane-system` |
| `SERVICEACCOUNT_NAME` | Explicit provider runtime ServiceAccount name, e.g. `provider-btp` |
| `IDENTITY_SUFFIX` | Fixed suffix used by this guide's example naming scheme, e.g. `workload.example.com` |
| `WORKLOAD_EMAIL` | Chosen IAS/BTP workload email; this guide uses `SERVICEACCOUNT_NAME@PROVIDER_NAMESPACE.IDENTITY_SUFFIX` as an example |
| `BTP_ORIGIN` | Exact BTP platform trust origin, e.g. `workload-platform` |
| `IAS_CONSUMER_CLIENT_ID` | Generated client ID of the confidential consumer created in step 3 |
| `DEPENDENCY_NAME` | Saved consumer dependency name, e.g. `btp-principal-propagation` |

Read both discovery documents and record issuer/JWKS/token endpoint values. An
IAS administration domain alias can differ from its advertised issuer: use
`IAS_ISSUER` for the projected token audience and the token endpoint's base for
`iasUrl`. Preserve exact scheme, path and trailing slash.

## 1. Prepare the IAS user and cluster trust

Use the [IAS administration documentation](https://help.sap.com/docs/cloud-identity-services)
for user/group creation and Corporate Identity Provider configuration. The
following are the settings needed by this provider, rather than a complete IAS
administration walkthrough.

### Workload user

In **Users & Authorizations**, create an active user with Email and Login Name
set to `WORKLOAD_EMAIL`. Select **Set status active** without an initial password
or activation email; choose User Type **None** where available. Create a dedicated
group and add only the intended workload user.

The example identifier `SERVICEACCOUNT_NAME@PROVIDER_NAMESPACE.IDENTITY_SUFFIX`
is a proposal, not a required format. Choose a distinct suffix per trusted cluster
or another scheme that keeps unrelated workloads separate. The NameID mapping,
IAS user, BTP grants and `workloadIdentity.userEmail` must agree.

### Corporate Identity Provider

Under **Identity Providers → Corporate Identity Providers**, create an **OpenID
Connect Compliant** IdP for the cluster. Configure these settings and save:

| IAS setting | Required configuration |
|---|---|
| OpenID Connect Configuration | Load the cluster discovery URL; verify `CLUSTER_ISSUER` and `CLUSTER_JWKS_URI` |
| Enriched Token Claims | Map `NameID` to the workload user's identifier, using the expression below for this guide's naming example |
| Identity Federation | Enable **Use Identity Authentication user store** and **Allow Identity Authentication users only** |
| Assigned User Groups | Select only the dedicated workload group; restrictions require IAS-users-only enabled |
| Forward All SSO Requests to Corporate IdP | Off for this JWT-only setup; preserve human administration login |

For [NameID claim enrichment](https://help.sap.com/docs/identity-authentication/identity-authentication/enrich-token-claims-coming-from-corporate-idp),
use this example and substitute the fixed cluster-specific suffix:

```text
${"kubernetes.io".serviceaccount.name}@${"kubernetes.io".namespace}.workload.example.com
```

Keep the quotes around the dotted `kubernetes.io` property. The result must equal
`WORKLOAD_EMAIL`. This mapping binds by ServiceAccount name, not UID; restrict who
can run pods as that SA or request its tokens. The outgoing application subject
can remain IAS **User ID**, with `mail` sourced from **Email**.

Use an appropriate JWKS refresh interval. A
Kubernetes issuer may lack browser authorization/token endpoints; do not invent
endpoints or add a secret to suppress a browser-SSO warning. For this setup,
**Apply Application Configurations** was off; review your tenant/application
policies separately. Interactive SSO validation does not prove JWT exchange.

## 2. Prepare BTP platform trust and roles

Follow [BTP platform trust and federation](https://help.sap.com/docs/btp/sap-business-technology-platform/trust-and-federation-with-identity-providers)
to establish trust with the IAS tenant at the **global-account platform** level.
Collect the resulting `BTP_ORIGIN` and assign the workload user's required role
collections under that exact origin.

- Global-account platform origins end in `-platform`. Application-only subaccount
  trust is insufficient for provider/CLI/CF administration. Verify the inherited
  target-subaccount trust; BTP and CF must agree on the configured origin.
- Assign roles for the intended operation set. **Global Account Administrator**
  and **Subaccount Administrator** are broad examples, not universal minimums.
  In the cockpit select the IAS origin; in CLI assignments include
  `--of-idp BTP_ORIGIN`. Omitting it selects a different identity (`sap.default`).
- In IAS, open **SAP Business Technology Platform → Trust → Trust Corporate
  Identity Providers → Edit**. Select the specific cluster IdP and save.
  Keep **Allow logon with all configured corporate
  identity providers** off. A visible IdP row alone is not selected trust.

BTP role assignment can create its shadow user; it does not create the IAS user.
Trust and role assignments are both prerequisites.

## 3. Prepare the IAS consumer for federated native user calls

Skip this section for native **service-client** mode. CLI/CF assertion login uses
the BTP integration; native **user-preserving CIS** additionally requires a
confidential OpenID Connect consumer application in IAS.

In the **IAS administration console**, under **Applications & Resources →
Applications**, configure these two applications:

- **Target application:** the existing **SAP Business Technology Platform**
  application under **Bundled Applications**, associated with your BTP platform
  trust. It provides the API and determines the BTP token audience.
- **Consumer application:** a separate OpenID Connect application that you create
  for the BTP provider workload. It trusts the cluster JWT and consumes that API.

### 3.1. Enable the API on the target BTP application

1. In the IAS administration console, open **Applications & Resources → Applications**.
2. In the application list, expand **Bundled Applications** and select **SAP
   Business Technology Platform**. If several BTP applications are listed, use
   the one associated with the platform trust from step 2.
3. In that application's **Trust** tab, select **Provided APIs** under
   **Application APIs**.
4. Enable **Allow all APIs for principal propagation** and select **Save**.

The target permission and consumer dependency are separate settings. The
dependency selects the target/API; it does not enable API exposure on the target.
Without this permission, IAS rejects the exchange with `invalid_target` and
`doesn't provide the requested api: principal-propagation`, even when workload
authentication succeeds and the consumer dependency exists.

Use the actual BTP application: its client ID must be the audience trusted by the
CIS binding's XSUAA service. This API permission is separate from **Allow logon
with all configured corporate identity providers**; keep that logon setting off
and retain the selected cluster IdP trust from step 2. BTP roles are still required.

### 3.2. Create the workload consumer application

1. Return to **Applications & Resources → Applications** and select **Create**.
2. Create an application with protocol **OpenID Connect** and a descriptive
   display name, for example **BTP provider workload**. This is a new consumer;
   keep the existing bundled BTP application as the target.
3. Select the newly created application in the list. In its **Trust** tab, open
   **OpenID Connect Configuration → Configure** and set a stable application
   name, for example `btp-provider-workload`.
4. Within **OpenID Connect Configuration**, open **Authentication → Grant Types
   → Edit**. Enable **JWT Bearer** and disable grants unused by this workload
   flow. Save the configuration. Browser redirect URLs are unnecessary for the
   JWT-bearer flow.

Keep this **consumer application selected** for sections 3.3–3.5. Their Trust-tab
settings belong to the consumer, not the bundled BTP application.

### 3.3. Configure JWT client authentication on the consumer

1. In the consumer's **Trust** tab, select **Client Authentication** under
   **Application APIs**.
2. Open **JSON Web Tokens → Configure Trust by Issuer → Add** and choose
   **Manual Configuration**.
3. Enter the following values:

| JWT client trust field | Value |
|---|---|
| Issuer | `CLUSTER_ISSUER` |
| JWKS URI | `CLUSTER_JWKS_URI` |
| Subject | `system:serviceaccount:PROVIDER_NAMESPACE:SERVICEACCOUNT_NAME` |
| Audience | Exact `IAS_ISSUER`, matching the projected token audience |

4. Choose an appropriate JWKS refresh interval. Save the issuer-trust dialog and
   then save the parent **Client Authentication** editor.
5. Keep **Enable Public Client Flows** disabled. Generate no IAS client secret or
   private signing key: the cluster's projected JWT authenticates this client.
6. Record the consumer's generated **Client ID** shown in **Client
   Authentication** as `IAS_CONSUMER_CLIENT_ID`.

See the official [JWT client authentication guide](https://help.sap.com/docs/cloud-identity-services/cloud-identity-services/auth-configure-jwt-for-oauth-client-authentication?locale=en-US)
for details of this editor.

### 3.4. Select the cluster IdP for user federation on the consumer

1. In the consumer's **Trust** tab, select **Trust Corporate Identity Providers**
   under **Conditional Authentication**, then select **Edit**.
2. Select the specific cluster Corporate IdP created in step 1. Keep **Allow
   logon with all configured corporate identity providers** off.
3. Select **Save**.

This user-federation trust is separate from JWT client authentication in section
3.3. Client authentication can succeed while user exchange fails if this IdP is
not selected.

### 3.5. Add the BTP API dependency on the consumer

1. In the consumer's **Trust** tab, select **Dependencies** under **Application
   APIs**.
2. Select the **APIs** tab and then **Add**.
3. Enter a **Dependency Name**, for example `btp-principal-propagation`; record it
   as `DEPENDENCY_NAME`.
4. In the **Application** dropdown, select the existing **SAP Business Technology
   Platform** target configured in section 3.1.
5. In the **API** dropdown, select **All APIs / Allow all APIs for principal
   propagation**, as advertised by that target. Select the actual dropdown entry.
6. Select **Save**.

See the official [API consumption/dependency guide](https://help.sap.com/docs/cloud-identity-services/cloud-identity-services/consume-api-from-provider-application)
for details of this editor.

### 3.6. Transfer the consumer identifiers to ProviderConfig

Use these values in the **Kubernetes ProviderConfig** shown in step 4:

| ProviderConfig field | Value |
|---|---|
| `spec.workloadIdentity.iasUrl` | `IAS_TOKEN_BASE_URL` |
| `spec.workloadIdentity.iasClientId` | `IAS_CONSUMER_CLIENT_ID` from the consumer's Client Authentication view |
| `spec.workloadIdentity.iasResource` | `urn:sap:identity:application:provider:name:DEPENDENCY_NAME`, for example `urn:sap:identity:application:provider:name:btp-principal-propagation` |

The consumer's client ID differs from its application ID and from the bundled
BTP application's client ID. Use the value recorded in section 3.3.

The provider submits the projected JWT as both user and client assertion. It
requests an IAS **access JWT addressed to BTP**, then exchanges it at the manual
CIS binding's XSUAA endpoint using its client ID/secret. A consumer-only audience
is insufficient. The provider checks IAS `mail` and CIS `user_name`/`origin`
against the configured identity; it needs no platform master client credentials.
A 15-minute local replacement interval does not define IAS server session policy.

## 4. Configure the runtime and ProviderConfig

Save the following runtime configuration, substituting the exact IAS issuer and
SA name. Apply it before selecting workload authentication:

```yaml
apiVersion: pkg.crossplane.io/v1beta1
kind: DeploymentRuntimeConfig
metadata:
  name: provider-btp-workload
spec:
  serviceAccountTemplate:
    metadata:
      name: provider-btp
  deploymentTemplate:
    spec:
      selector: {}
      template:
        spec:
          containers:
            - name: package-runtime
              volumeMounts:
                - name: workload-assertion
                  mountPath: /var/run/secrets/workload
                  readOnly: true
          volumes:
            - name: workload-assertion
              projected:
                sources:
                  - serviceAccountToken:
                      audience: IAS_ISSUER
                      expirationSeconds: 600
                      path: token
---
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-btp
spec:
  package: ghcr.io/sap/crossplane-provider-btp/crossplane/provider-btp:WORKLOAD_IDENTITY_VERSION
  runtimeConfigRef:
    name: provider-btp-workload
```

`serviceAccountTemplate` fixes the SA name; the actual namespace comes from the
provider deployment. It must match the IAS user mapping and client subject. Mount
without `subPath` so kubelet projection rotation stays visible. Preserve other
runtime settings and the Kubernetes API credential when adapting an existing
managed installation; modify its owning configuration if another operator manages it.
Use this **federated user** ProviderConfig:

```yaml
apiVersion: btp.sap.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: workload
spec:
  globalAccount: GA_SUBDOMAIN
  cliServerUrl: BTP_CLI_URL
  cisCredentials:
    source: Secret
    secretRef:
      namespace: default
      name: cis-provider-secret
      key: data
  workloadIdentity:
    tokenFile: /var/run/secrets/workload/token
    identityProvider: BTP_ORIGIN
    userEmail: WORKLOAD_EMAIL
    iasUrl: IAS_TOKEN_BASE_URL
    iasClientId: IAS_CONSUMER_CLIENT_ID
    iasResource: urn:sap:identity:application:provider:name:btp-principal-propagation
```

Use your CIS Secret name, namespace and key. All three IAS fields must be provided together.
For **service-client** native mode, use `client_credentials` CIS bindings and omit
all three IAS fields together. Both modes keep `tokenFile`, `identityProvider` and
`userEmail` for CLI/CF. Omit `serviceAccountSecret`: workload mode rejects mixed
password credential inputs. Never put the JWT itself in a CR or Secret. Workload
settings are accepted only from `spec.workloadIdentity`. Legacy user credential
JSON rejects `TokenFile`, `IASURL`, `IASClientID`, `IASResource` and
`workloadIdentity` keys regardless of casing; they cannot enable workload
assertion reads or exchanges through a Secret. Other legacy credential fields
and unrelated unknown metadata retain their existing parsing behavior.

For migration, create a separate ProviderConfig and switch the existing resources'
references to it. Retain old credentials/configuration for rollback. Removing `workloadIdentity` alone is not a rollback: the legacy
`serviceAccountSecret` reference must also be restored. Existing CIS user-token
bindings can be retained if compatible with this exchange; validate their actual
plan/tenant/grant configuration before migrating.

## Use the workload email for Subaccount administrators

When [creating a Subaccount](../account/subaccount.mdx), use the chosen
`WORKLOAD_EMAIL` in `subaccountAdmins`, rather than the raw Kubernetes subject:

```yaml
apiVersion: account.btp.sap.crossplane.io/v1alpha1
kind: Subaccount
metadata:
  name: my-subaccount
spec:
  providerConfigRef:
    name: workload
  forProvider:
    displayName: My subaccount
    region: BTP_REGION
    subdomain: UNIQUE_SUBDOMAIN
    subaccountAdmins:
      - WORKLOAD_EMAIL
```

## Changing the workload principal

Restart the provider after changing the trusted workload's issuer, ServiceAccount
subject, audience, or its IAS/BTP identity mapping. Update the IAS trust, BTP role
assignments and ProviderConfig together before the restart. Native CIS pins the
principal of a cached token source and rejects a different principal with
`native CIS workload principal changed; restart required`. Restarting clears the
in-memory clients and authenticates the new identity.

Normal kubelet token rotation for the same issuer, subject and audience does not
require a restart.

## Troubleshooting

| Symptom | Check |
|---|---|
| Workload fields rejected/pruned | Installed version and ProviderConfig CRD contain the feature |
| Cannot read assertion / expired claims | Token mount/path, SA identity, kubelet rotation and clock |
| IAS client authentication fails | Exact issuer/JWKS/subject/audience in consumer JWT client trust; confidential client ID |
| `Invalid trust configured for token issuer` | Consumer's Corporate IdP checkbox is actually selected and saved; client auth may succeed independently |
| IAS `invalid_target`: target does not provide `principal-propagation` | On the target BTP application, enable and save **Trust → Provided APIs → Allow all APIs for principal propagation**; then verify the consumer dependency targets that application/API |
| XSUAA says parties are missing from audience | Saved BTP dependency and `iasResource`; use IAS access JWT addressed to BTP |
| Provider rejects email/origin mismatch | IAS NameID lookup, user mail mapping, configured email and exact BTP origin |
| Native API 401/403 | Binding central/local plan, token tenant/audience, user's target roles; token issuance alone is not authorization proof |
| CF login succeeds but manager calls fail | Correct CF org roles/origin; GA roles alone do not grant CF org membership |
| ServiceInstance plan lookup fails | Service Manager credential is still required for name-based resolution; a pre-resolved plan ID skips lookup |

## Reference documentation

- [BTP platform trust and federation](https://help.sap.com/docs/btp/sap-business-technology-platform/trust-and-federation-with-identity-providers)
- [CIS service plans](https://help.sap.com/docs/btp/sap-business-technology-platform/a508b724bf6d457ca7ac024b8e4b8457.html)
- [IAS NameID claim enrichment](https://help.sap.com/docs/identity-authentication/identity-authentication/enrich-token-claims-coming-from-corporate-idp)
- [IAS JWT client authentication](https://help.sap.com/docs/cloud-identity-services/cloud-identity-services/auth-configure-jwt-for-oauth-client-authentication?locale=en-US)
- [IAS API consumption and dependencies](https://help.sap.com/docs/cloud-identity-services/cloud-identity-services/consume-api-from-provider-application)
- [Contributor authentication and cache notes](../../contribution-notes/workload-identity.md)
