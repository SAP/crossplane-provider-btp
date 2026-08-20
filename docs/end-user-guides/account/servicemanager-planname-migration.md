# ServiceManager: Migrating from `service-operator-access` to `subaccount-admin`

## Background

As of this release, the default `planName` for the `ServiceManager` resource has changed from
`service-operator-access` to `subaccount-admin`.

The `service-operator-access` plan is intended exclusively for the
[SAP BTP Service Operator](https://github.com/SAP/sap-btp-service-operator) and carries
assumptions about how credentials are consumed that do not apply to general crossplane usage.
`subaccount-admin` is the more appropriate default for provider-btp.

## Who is affected?

**Existing `ServiceManager` resources are not affected.** `planName` is immutable once set — the
field is pinned at creation time and cannot be changed on a running resource. Your existing
resources continue to use whatever plan they were created with.

You are only affected if you:

1. Rely on the implicit default (i.e. you omit `planName` from your `ServiceManager` manifest) **and**
2. You need the resource to keep using `service-operator-access`.

## What do I need to do?

### If you need `service-operator-access` going forward

Set it explicitly in your manifest:

```yaml
apiVersion: account.btp.sap.crossplane.io/v1beta1
kind: ServiceManager
metadata:
  name: my-service-manager
spec:
  forProvider:
    subaccountRef:
      name: my-subaccount
    planName: service-operator-access
```

### If you want to migrate an existing resource to `subaccount-admin`

Because `planName` is immutable, you must delete and recreate the resource:

1. Note the current `subaccountGuid`, `serviceInstanceName`, and `serviceBindingName` from the existing resource.
2. Delete the existing `ServiceManager` resource (this deletes the BTP service instance and binding).
3. Recreate it with `planName: subaccount-admin` (or omit `planName` to use the new default).

> **Warning:** Deleting a `ServiceManager` resource deletes the underlying BTP service instance and
> binding. Any downstream resources depending on the credentials written to the connection secret
> will lose access until the new resource is ready.

### If you are creating new resources

No action needed — new `ServiceManager` resources without an explicit `planName` will now default
to `subaccount-admin`.
