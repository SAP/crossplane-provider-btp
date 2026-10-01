---
sidebar_position: 4
---

# Import by Lookup

Instead of finding a resource's ID and setting `crossplane.io/external-name` yourself, you can ask the provider to find it. Set the lookup annotation on the managed resource:

```yaml
metadata:
  annotations:
    btp.sap.crossplane.io/lookup: "true"
```

While the resource has no identifier yet, the provider looks it up in BTP by its natural key (a name, a subdomain, ...). Then:

- **Exactly one match that agrees with the spec:** the provider adopts it by writing its ID to `crossplane.io/external-name` and records an `ExternalNameAdopted` event naming the key that found it.
- **No match:** the provider creates the resource as usual. The annotation means "import it if it already exists".
- **A match that disagrees with the spec** (for example, a service instance with the same name on a different plan): the provider refuses to adopt it and does not create anything. The resource reports the reason until you fix the spec or remove the annotation.
- **More than one match, or a failed lookup:** the provider reports an error and retries. It never picks one match at random and never creates a resource because a lookup failed.

An identifier you set yourself always wins. The lookup runs only while the resource has none, and never while it is being deleted:

| `crossplane.io/external-name` | Lookup runs? |
| ----------------------------- | ------------ |
| not set | yes |
| equal to `metadata.name`, and that name is not a GUID (left by Crossplane's default initializer or older provider versions) | yes |
| equal to `metadata.name`, and that name is a GUID | no, it is taken as the identifier |
| any other value (a GUID, or a compound key such as `instanceID/bindingID`) | no |

The only valid annotation values are `"true"` and `"false"`; any other value is reported as an error.

## Supported Resources

| Kind | Looked up by | Must also match |
| ---- | ------------ | --------------- |
| `ServiceInstance` | `spec.forProvider.name` in the subaccount | the service plan that `offeringName`/`planName` (or `servicePlanID`) resolve to at the time of the lookup |
| `ServiceBinding` | `spec.forProvider.name` under the referenced service instance | — |
| `Subaccount` | `spec.forProvider.subdomain` in the global account | `spec.forProvider.region` |
| `ServiceManager` | `serviceInstanceName` and `serviceBindingName` (default `managed-service-manager` and `managed-service-manager-binding`) | `planName` (default `subaccount-admin`) |
| `CloudManagement` | `serviceInstanceName` and `serviceBindingName` (default `managed-cloud-management` and `managed-cloud-management-binding`) | the `cis/local` plan |
| `KymaEnvironment` | environment name in the subaccount | `spec.forProvider.planName` |
| `CloudFoundryEnvironment` | the subaccount's Cloud Foundry environment (BTP allows one per subaccount) | `orgName`, `environmentName` and `landscape`, each only when the spec sets it (the provider never updates a Cloud Foundry environment, so a difference could never be fixed later) |
| `Subscription` | `appName/planName` | — |

For `ServiceManager` and `CloudManagement`, an existing instance without its binding is adopted and the provider creates the missing binding.

Service instance names are unique in a subaccount. An instance with the declared name that runs on another plan can therefore neither be adopted nor make room for a new one: declare the plan it runs on to adopt it, or choose another name to create a new instance.

The annotation has no effect on any other kind.

## Unsupported Combinations

- **`ServiceBinding` with `spec.rotation` set.** A rotating binding lives under a generated `<name>-<suffix>` name, and rotation rewrites its external-name, so a lookup by spec name cannot identify it. The provider reports an error instead. To import a rotating binding, set `crossplane.io/external-name` to its ID.
- **`ServiceBinding` whose service instance is not resolved yet.** The lookup is scoped to the parent instance; the provider waits for it rather than creating a binding.

## What Adoption Means

An adopted resource is managed exactly like one the provider created. With `Update` in `managementPolicies` the provider changes it to match the spec, and deleting the managed resource with `deletionPolicy: Delete` deletes it in BTP.

Several managed resources can adopt the same BTP resource. That is fine while they only observe it, but any one of them with `Delete` deletes it for all of them, and several with `Update` that disagree on the spec keep overwriting each other.

## Check Before You Hand Over Control

To verify what a lookup matches before the provider changes anything, adopt in observe-only mode first:

```yaml
apiVersion: account.btp.sap.crossplane.io/v1alpha1
kind: ServiceInstance
metadata:
  name: my-instance
  annotations:
    btp.sap.crossplane.io/lookup: "true"
spec:
  managementPolicies: ["Observe"]
  forProvider:
    name: my-instance
    offeringName: destination
    planName: lite
    serviceManagerRef:
      name: my-service-manager
    subaccountRef:
      name: my-subaccount
```

Check the `ExternalNameAdopted` event and `status.atProvider`, then switch `managementPolicies` to `["*"]`.

## Using Lookup in Compositions

Don't hard-code the annotation in a composition: every composite resource rendered from it would then adopt whatever matches, without the person creating it choosing to. Expose the decision on the composite resource instead, so adoption stays an explicit choice of whoever creates it.

A pattern that works well is two required fields on the composite resource:

| Field | Values | Effect on the composed BTP resources |
| ----- | ------ | ------------------------------------ |
| `resourceAccessMode` | `FullControl`, `PreventDelete`, `ObserveOnly` | `managementPolicies` of `["*"]`, `["Observe", "Create", "Update", "LateInitialize"]` or `["Observe"]` |
| `existingResourcePolicy` | `Adopt`, `Fail` | `Adopt` sets the lookup annotation; `Fail` leaves it off, so an existing resource with the same name is a conflict |

The fields in the composite resource definition:

```yaml
parameters:
  type: object
  required: [resourceAccessMode, existingResourcePolicy]
  properties:
    resourceAccessMode:
      type: string
      enum: [FullControl, PreventDelete, ObserveOnly]
    existingResourcePolicy:
      type: string
      description: |
        What to do when a BTP resource with the declared name already exists:
        Adopt takes it over, Fail treats it as a conflict.
      enum: [Adopt, Fail]
```

And a composition step that applies them, using `function-go-templating`:

```yaml
pipeline:
  - step: render
    functionRef:
      name: function-go-templating
    input:
      apiVersion: gotemplating.fn.crossplane.io/v1beta1
      kind: GoTemplate
      source: Inline
      inline:
        template: |
            {{- $params := .observed.composite.resource.spec.parameters }}
            {{- $policies := dict "FullControl" (list "*") "PreventDelete" (list "Observe" "Create" "Update" "LateInitialize") "ObserveOnly" (list "Observe") }}
            apiVersion: account.btp.sap.crossplane.io/v1alpha1
            kind: ServiceInstance
            metadata:
              annotations:
                gotemplating.fn.crossplane.io/composition-resource-name: instance
                {{- if eq $params.existingResourcePolicy "Adopt" }}
                btp.sap.crossplane.io/lookup: "true"
                {{- end }}
            spec:
              managementPolicies: {{ get $policies $params.resourceAccessMode | toJson }}
              forProvider:
                name: {{ $params.instanceName }}
                offeringName: hana-cloud
                planName: hana
                subaccountRef:
                  name: {{ $params.subaccountName }}
                serviceManagerRef:
                  name: {{ $params.serviceManagerName }}
```

Choosing `existingResourcePolicy` over a resource's life:

- **Fresh subaccount:** use `Adopt`. Nothing exists there yet that a name could collide with, and if the control plane is ever rebuilt and the external-names are lost, the composed resources re-attach to their BTP resources instead of conflicting with them.
- **Subaccount that already holds other resources:** start with `Fail`, so an accidental name collision shows up as an error instead of silently taking over an unrelated resource. Switch to `Adopt` once your resources are provisioned.
- **Resources created outside Crossplane:** use `Adopt` from the start, with `ObserveOnly` first (see the previous section) and `FullControl` or `PreventDelete` once the result looks right.
