# Service instance parameter readback

The native `ServiceInstance` controller maps each reconcile to a fresh internal
Terraform resource. Upjet stores Terraform state in memory and reconstructs it
from that resource when its operation cache is empty. Because the mapped
resource has no observation, reconstruction can copy the current desired
parameters into prior state before they have been applied.

Terraform BTP normally retains nonempty prior parameters during `Read`, even
when the broker returns different parameters. This can hide pending updates
after a provider restart and prevent correction of external parameter drift.

The embedded provider adapts `btp_subaccount_service_instance` so its `Read`
uses the existing Terraform import readback path for parameters. Identity,
health, authentication, and CRUD continue to use the Terraform implementation.
Upjet can then plan against broker parameters, apply missing desired fields,
and verify them on the next observation without depending on cached state.

Planning compares configured JSON object fields recursively. Broker-added
defaults are allowed; arrays and scalar values must match exactly. Omitted
fields are unmanaged, so omission does not request deletion of a broker field.
Parameter values are not copied into annotations or logs by this adapter.

This requires successful parameter retrieval. Terraform BTP's CLI facade treats
unavailable retrieval as missing parameters, including offerings that do not
support retrieval. The adapter preserves prior parameters in that case to
retain compatibility. It does not provide authoritative drift detection for
non-retrievable parameters, or distinguish temporary retrieval failure from an
unsupported offering. A completely empty returned object is also treated as
unavailable by the upstream facade; a nonempty object containing an empty
nested policy is read back normally.

The offline regression uses the real Terraform BTP provider and Upjet connector
with a stub CLI server and new operation store. It verifies an unapplied update
is detected after cache loss, ordinary Update repairs it, a fresh connector
verifies the result, broker defaults converge, and later external drift is
detected after another restart.
