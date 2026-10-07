# Service instance parameter readback

The native `ServiceInstance` controller maps each reconcile to a fresh internal
Terraform resource. Upjet stores Terraform state in memory and reconstructs it
from that resource when its operation cache is empty. Because the mapped
resource has no observation, reconstruction can copy the current desired
parameters into prior state before they have been applied.

The embedded provider adapts `btp_subaccount_service_instance` so `Read` uses
fresh broker parameter responses. A request-scoped capture in the existing
HTTP transport accepts both plain JSON objects and `data` envelopes, including
empty objects. This avoids the upstream plain-object decoder, which otherwise
silently drops parameters. Authentication, identity, health and CRUD use the
existing Terraform implementation. No additional client or login is created.

Upjet plans against observed parameters, applies missing desired fields and
verifies them on the next observation without depending on cached state.
Configured object fields are compared recursively; broker-added defaults are
allowed. Arrays and scalar values must match. Omitted fields are unmanaged,
so omission does not request deletion of a broker field. Numbers are compared
without rounding large integers through float64.

Missing, failed or malformed parameter readback produces an observation error
when parameter fields are configured. It never falls back to reconstructed
current desired values as proof of convergence. A successful empty object is
actual readback and can produce drift. Parameter values are not persisted in
annotations or added to error diagnostics by the adapter.

## Write-only offerings

This changes the default for configured parameters: successful readback is
required. Offerings that cannot return parameters need an explicit opt-out on
the native ServiceInstance:

```yaml
metadata:
  annotations:
    serviceinstance.account.btp.crossplane.io/parameter-readback: "false"
```

This opts out of broker verification and uses existing Terraform configuration
tracking. It cannot guarantee recovery of unapplied parameter updates after
cache loss or detect external parameter drift. It should only be used for
known write-only offerings, never to hide a transient retrieval failure.
ServiceInstances without configured parameter fields require no verification.

The offline regression uses the real Terraform BTP provider and Upjet connector
with a stub CLI server and fresh operation stores. The unchanged-provider
control reproduces false convergence with the plain response shape. The fixed
path applies and verifies a legacy display-name plus three pipeline policy
changes, repairs a policy-only mismatch, converges with broker defaults and
wrapped responses, recognizes empty objects as drift, and rejects failed or
malformed retrieval. An explicit write-only compatibility case is also tested.
