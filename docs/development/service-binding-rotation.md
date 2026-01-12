# Service Binding Rotation

> Secret rotation refers to the automated or manual process of replacing sensitive credentials—such as API keys, access tokens, passwords, and cryptographic keys—on a regular or policy-driven basis. In the context of cybersecurity and Non-Human Identities (NHIs), secret rotation ensures that machine credentials are periodically updated to reduce the risk of unauthorized access, credential theft, or long-lived exposure. The process typically involves generating a new secret, validating its integration, retiring the old secret, and updating dependent systems accordingly[^1].

[^1]: https://www.oasis.security/glossary/secret-rotation

## Problem

Secret Rotation should be solved in a standardized manner for all Cloud Orchestrator Crossplane providers to prevent many individual solutions. The API and behaviour should be as similar as possible between all implementations to mitigate false expectations when using multiple Crossplane providers where secret rotation is implemented.

## Definitions

- "Instance": One entity of the resource we wanna manage / rotate. It translates to exactly one entity on the server side (not to be confused with e.g. `BTP ServiceInstance`)
- "Current Instance": The instance that is the newest one and the one where the current connectionDetails come from
- "Retired Instances": The instances that still exist, but should no longer be used. They are kept in an array in the status (e.g. in `.status.retired`). Dependend applications should migrate to the (new) current instance
- "Expired Instances": Instances in `.status.retired` for which `expireAt` is in the past. They should be deleted as soon as possible (in the next reconcilation)
- "Enabling rotation": Definining `ttl` and `frequency` in `.spec.rotation`
- "Check if rotation is enabled": Check if `.spec.rotation` exists
- "Disabling rotation": Removing `.spec.rotation`
- "Current lifetime of an instance": How long the instance is valid since (`now()` - `.status.atProvider.createdAt`)

## API

### Spec

The configuration and usage for the end-user should be made as simple and straight forward as possible, while the logic is handled by the controller internally. This is why the rotation can be configured by just two paramters in `.spec.rotation`, namely `ttl` (time to live) and `frequency`. Both `ttl` and `frequency` are required fields. If `.spec.rotation` is not defined, then the rotation mechanism is considered disabled. Example spec:

```yaml
apiVersion: account.btp.sap.crossplane.io/v1alpha1
kind: ServiceBinding
metadata:
  name: (...)
spec:
  rotation:
    ttl: 336h # 14 days
    frequency: 228h # 12 days
  forProvider:
    name: my-sample
```

`ttl` describes how long one instance is valid. `frequency` means the duration after what livetime of the current instance a new instance should be created. This means that if e.g. an instance "A" is valid for 14 days, and the frequency  is set to 12 days, then 12 days after its creation, the current instance "A" gets retired and a new instance "B" gets created. This newly created instance "B" becomes the "current instance", but the other, now retired, instance "A", will still be valid for another 2 days. This period can now be used by depending applications to now use the new current instance. After these 2 days passed, instance "B" lived now for 14 days and will be deleted. At this point of time, instance "B" is already 2 days old, meaning it will now only take another 10 days for another instance "C" to be created.

### Status

The `.status.retiredKeys` field of the resource is the only place where information about retired instances  are stored (information about the current instance is located in `.status.atProvider`). `.status.retiredInstances` is an array, where every entry represents one retired instance. The order of this array is not important. Example status:

```yaml

apiVersion: account.btp.sap.crossplane.io/v1alpha1
kind: ServiceBinding
metadata:
  name: (...)
spec:
  rotation:
    ttl: 336h # 14 days
    frequency: 228h # 12 days
  forProvider: 
    name: my-sample
status:
  atProvider:
    name: my-sample-a4f3a2
    id: 59883485-9a21-4559-b8cf-a8890485cb4b
    otherField1: (...)
    otherField2: (...)
    id: f71a065b-e7c6-45e0-bef8-2fa93e5e9157
    name: demo-1-0b8rn
    createdDate: 2025-11-05T13:14:28Z
  retiredKeys:
    - name: my-sample-a4f3a2
      id: ca23c6d0-6c17-44b8-ab50-1c29fb049c83
      createdDate: 2025-11-05T12:04:16Z
      retiredDate: 2025-11-05T13:16:28Z
      deletionDate: 2025-11-05T13:14:28Z
```

The `.status.atProvider` can be filled with any arbitrary information about the current instance, just like it is done for other resources. Only a `.createdDate` field is required, to track when the instance should be retired. The `.status.retiredKeys` field is an array for the retired instances. It should only have the fields necessary for instance identification and rotation logic. These fields are: `.createdAt` (when the instance got created), `.retiredDate` (when the instance got retired), `.deletionDate` (when the instance is planned for deletion), and `.id` (the external identifier) and, in the case of upjetted resources, `.name`.

The `.status.atProvider` field as well as the connection details (the content of the secret associated with the resource) should always represent the current instance. This means when a new instance is created during rotation, the atProvider field and the content of the connection details secret should be updated accordingly. However, as new instances can potentially take some time until being ready, the current atProvider and connection details fields should not change. This means that the atProvider field and the connection details should not suddenly be empty, even for a short period of time. This also means that during this short time, the "just retired" instance may be represented in the atProvider fields as well as in the `.retiredKeys` array. This duplication is not a problem.

The `retiredKeys` field is an array because `.spec.rotation.{ttl,frequency}` can be chosen in a way that makes it possible to have more than one retiredKey at a time. To prevent potential senseless configurations like `{ttl: 24h, frequency: 1m}` (which would result in up to 360 instances existing at the same time), a warning is displayed by the controller if the configuration could lead to more than one (1) instance being retired at the same time.

## Reconcilation Logic

The rotation logic is splitted into the designated parts of a reconcilation.
The following subchapters describe only the logic necessary for the rotation. 
Logic for the normal behaviour of the resource should take place always before the rotation logic.

### Observe

Here is decided rather 1) the current instance should be retired and a new instance should be created, 2) retired instances should be deleted, or 3) everything is fine for now, in this order.

1) If the lifetime of the current instance is longer than the duration of `.spec.rotation.frequency`, the rotation should start. For this, the current instance is added to the `.status.retireKeys` array with it's respective `.createdAt`, `.id` and `.name` attributes and `.retiredKeys` (=`now()`) and `.deletionDate` (=`.createdDate+.spec.rotation.ttl`). Aftrwards, return with `ResourceExists: false`. This will immediately trigger a creation of the new instance in `Create()`.

2) If the instance is still valid, potentially expired resources can be deleted. This should also happen if the user deactivated rotation in the meantime. For this, each instance in `.retiredKeys` is checked independently. First, the `.deletionDate` of the instance should be updated. If in the mean time the user increased `.spec.rotation.ttl`, the `.deletionDate` value should be adjusted accordingly. This change should also be saved back to the CR. `.deletionDate` is only needed in the case that the user deactivates the future rotation by removing the `.spec.rotation` field, which makes `.spec.rotation.ttl` not accessible in the future. In this case, currently retired instances should still be removed according to the old `.spec.rotation.ttl` behaviour, and the `.deletionDate` field is needed to save this information. It also doubles as a nice informative view for the enduser. If there are any instances where `.deletionDate>now()`, call the `Update()` reconcilation function by returning `ResourceUpToDate: false`.

3) If no early return because of the two checks have happened, there are no other checks needed. Continue with the normal reconcilation. This includes updating the `.status.atProvider` fields, the content of the connection secret and a drift-detection check. If the instance has been rotated in a previous reconcilation call, this content may change, which is desired behaviour. The connection secret should always represent the data of the current instance.

### Create

The `Create()` part of the reoncilation should not distinguish between the creation of the first resource or the creation as part of the rotation cycle. After creating an instance (in most cases a unique name is needed here, e.g. through a random suffix), the `crossplane.io/external-name` annotation should be set with the new value. The regular `Observe()` call will handle the part of updating the `.status.atProvider` field.

### Update

The deletion of expired instances is seen as an update of the resource, and therefore happens in the `Update()` part of the reconcilation, after the regular operations that are part of the `Update()` process. This could end in potential false-positive update calls, which is assumed to be okay.

For every instance in `.retiredKeys`, check if `.deletionDate>now()`. If true the instance should be deleted. If any of the instances in `.retiredKeys` experience an error during deletion, it should not be diretly returned back, because there could be other instances afterwards that could be deleted successfully. So instead, cache errors and return them united afterwards.

### Delete

When the resource is marked for deletion, it should beginn with the deletion of the retired instances before deleting the currently valid instance, in case of an error. Similar to how expired instances are deleted in the `Update()` part of the reconcilation, it should be iterated through `.retiredKeys` and all remaining instances should be deleted (in contrast to only the expired instances like in `Update()`).

If this step was successful, continue with the deletion of the current instance.

