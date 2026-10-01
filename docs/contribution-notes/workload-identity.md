# Workload identity for Terraform/CLI authentication

This opt-in mode replaces the BTP user's password for Terraform-backed controllers, including the internal Terraform connectors used by ServiceInstance and ServiceBinding. It uses the Terraform provider's assertion login. Native CIS clients still require a manually provisioned CIS central binding Secret; this feature does not create Cloud Management instances, bindings, or credentials.

## Setup

1. Configure IAS workload federation for the cluster's OIDC issuer and JWKS. Restrict admission to the intended namespace/ServiceAccount and map it to a dedicated IAS user. Establish BTP platform trust and assign the BTP roles needed by the operations. Trust establishes authentication; role assignments establish authorization.
2. Provision a CIS central instance/binding outside this feature, with instance parameter `grantType=clientCredentials`. Install its credentials as a Kubernetes Secret using the existing setup documentation. Native clients require `grant_type=client_credentials`; user/password CIS grants are rejected in workload mode. This change does not add native X.509 support or CIS renewal.
3. Configure DeploymentRuntimeConfig to mount a projected ServiceAccount token. Audience must match the IAS advertised issuer, expirationSeconds can request 600, and the token must be mounted without subPath so kubelet rotation is visible. Keep Kubernetes API credentials separately.
4. Configure ProviderConfig with the manual CIS Secret reference and workload identity:

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

Omit serviceAccountSecret in workload mode. Existing password configurations remain supported when workloadIdentity is absent. Never put the JWT in ProviderConfig, environment variables, examples, or logs. The setup reads the mounted file on each reconciliation and passes `assertion` plus `idp` to the Terraform provider; no username/password is supplied.

## Authentication boundaries and lifecycle

Terraform sessions are cached by GA/CLI/IdP configuration and public assertion issuer/subject, independently of JWT rotation. Cache entries are replaced after 15 minutes; HTTP/backend 401 evicts them for a fresh login on the next reconciliation. This bounds local session reuse, not server session validity/revocation. Projected JWT expiry, IAS enforcement, and server session lifetime are separate concerns. Assertions are not part of the workload cache key. This feature does not change BTP CLI server refresh-token behavior or implement proactive logout.

Native clients keep reading cisCredentials. Workload userEmail/idp supplies non-secret user metadata, for example Kyma creation. Cloud Foundry organization manager reads/updates still use direct CF username/password authentication and return a clear unsupported-operation error when credentials are absent. This feature does not make every native resource passwordless or remove the manually supplied CIS client secret.

ServiceInstance name-based plan resolution still needs a Service Manager Secret. A pre-resolved servicePlanID skips that lookup. ServiceInstance/ServiceBinding recovery paths may still use the manual CIS credential. Preserve manual CIS credentials until dependent resources are deleted. SI/SB ownership, rotation policies, and deletion behavior are unchanged; automatic CIS bootstrap is a separate project.

## Validation

Tests exercise both setup paths without reading any Secret, refreshed token reads, rejection of mixed user credential inputs, real Terraform protocol-v5 assertion login with no username/password, cache reuse across JWT rotation, eviction/recycle reauthentication, and native email metadata. Existing password setup/cache tests remain applicable. Live verification should create/delete SI/SB CRs with a manual CIS Secret, restart the provider, and repeat using a newly issued projected token. Do not infer real expiry recovery from restart alone.
