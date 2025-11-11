# Migration Handling

## Context
Sometimes resource API specs, lookup parameters and requirements for a Managed Resource change *BUT* someone is always relying on the previous version already. Across providers we lacke a unified definiton and process to handle such changes. This ADR proposes a consistent process for deciding on and managing migrations. 

## Background
Dev Sync (04.11.2025):

From Crossplane standpoint this is okay
We do not use it usually modify forProvider in the Observe
Traditionally this is filled in via a Webhook
We consider this is a migration -> one time changes should not be done in Observe
=> This goes into a broader migration ADR direction, how do we generally handle such cases?

=> If name is empty we use metadata.name, we do not update the spec as of now.
# ADR: Migration Strategies for Crossplane Providers

## Context
Resource API specs, lookup parameters, and Managed Resource requirements change over time while existing resources rely on previous versions. A unified approach for handling migrations is needed.

## Problem
Migrations affect both new and existing resources when changes are not purely additive. One-time transformations are required to move resources from prior to posterior state.

## Migration Approaches

### 1. Webhooks
Mutate resource specs at admission time. Traditional approach for field transformations.
- **When to use**: Complex field restructuring, adding required fields
- **Pros**: Clean separation, runs before controller logic
- **Cons**: Additional infrastructure, webhook availability concerns

### 2. LateInitialize in Observe()
Populate missing fields during observation phase using external resource state.
- **When to use**: Backwards compatibility, setting defaults from external state
- **Pros**: Simple implementation, no additional components
- **Cons**: Should not modify `spec.forProvider` (anti-pattern)

### 3. Kubebuilder Validations
Prevent invalid field combinations or enforce immutability at API level.
- **When to use**: Preventing updates to immutable fields, format validation
- **Pros**: Fails fast, clear error messages
- **Cons**: Limited to validation, cannot transform data

### 4. Controller Logic
Handle migration within controller reconcile loops.
- **When to use**: State-dependent migrations, complex business logic
- **Pros**: Full access to resource state and external APIs
- **Cons**: Can complicate controller logic

## Decision Matrix

| Scenario | Approach | Rationale |
|----------|----------|-----------|
| Add required field | Webhook + LateInitialize | Webhook for new, LateInitialize for existing |
| Immutable field enforcement | Kubebuilder validation | Prevents invalid updates at API level |
| Field format changes | Webhook | Clean transformation at admission |
| Backwards compatibility | LateInitialize (status only) | Populate from external state without spec modification |
| Complex state migrations | Controller logic | Requires external API context |

## Guidelines

1. **Avoid modifying `spec.forProvider` in Observe()** - violates Crossplane patterns
2. **Use webhooks for one-time transformations** - cleaner than controller logic
3. **Prefer validation over transformation** when possible
4. **Document migration paths** clearly for users and developers
5. **Test both new and existing resource scenarios**

## References
- [Crossplane Managed Resource Design](https://github.com/crossplane/crossplane/blob/main/design/one-pager-managed-resource-api-design.md)
- [Crossplane Import Guide](https://docs.crossplane.io/latest/guides/import-existing-resources/)

### Definitions

Migrations are a concern when changing anything related to the Kubernetes resource spec or logic within the controller.
Based on this definition, common assumptions are:
1. Resources have a state prior and posterior to their migration. [1]
2. As long as a feature is not purely additive, it affects both new and existing resources. [2]
3. There is a clear one time process which when applied to the prior state of a resource leading to its posterior state. 

### Default Behavior in Crossplane
- By default, the external name is initialized (by crossplane-runtime) to the Kubernetes `metadata.name` of the Managed Resource (MR) if not explicitly set.
- This default conflicts with cases where the external system uses a different unique identifier.

### Importing Existing External Resources (Official Flow) [3]
1. Create an MR manifest with `spec.managementPolicies: Observe`.
2. Add the external-name annotation with the external resource identifier. If not globally unique, also supply distinguishing `spec.forProvider` fields.
3. Apply the MR. Controller populates `status.atProvider`.
4. Copy the fields from `status.atProvider` to `spec.forProvider` and change `managementPolicies` to `*` to gain full control.

## Our Approach

### Definition
The external-name (string) identifies the external resource in the external system. 
A provider can adopt two qualities:

(1) The existance of the external-name express that there should be an external resource it identifies. So if the user creates a MR with the external-name set, the user explicitly states that this resource should exists in the external system (to its best knowledge) and should be adopted/matched. This prevents unintended adoption/matching of existing resources. This is also an advantage over capturing user intent via managment policies, as they would not prevent unintended matching of existing resources. (2) The identifier than contains the information the provider needs to identify the external resource. This can be an unique identifier or a compound key.

Exceptions to this rule might be necessary and have to be stated clearly for the user and developer.

In Create(), the external name is set based on the response of the API (if possible). Observe() use the external-name to match the external resource, Update() and Delete() also use the external-name. [4]
This is easy in the case of a unique identifier. If we have a compound key, we deconstruct the compound key to perform the API requests.

### Special Case: defaulting of external-name
If external-name is unset, crossplane-runtime will run a default initializer that will set the external name to metadata.name before the first Observe() is called.

This poses two problems:
- If we set any kind of initializer ourselves later, we don’t get the default initializer automatically and therefore might run into bugs if we don’t check for empty initializer
- we loose the ability to detect the intent of the user in setting the external-name explicitly (part 1 of our Definition)

Consequently, we have to remove this initializer from running. We will have our own collection of default initializers that contain the set of crossplane default initializers without the external-name defaulting.

### Implementation guideline

#### Observe()
1. Check if external-name is empty.
   - If we need backwards compatibility, we create our API query from the spec.forProvider fields.
     - If resource exists we perform the normal needsCreation/needsUpdate and set the external name to the correct value.
     - If resource does not exist, it needsCreation.
   - Else no backwards compatibility needed, return resourceExist: false.
2. External-name is set, we check its format.
   - If format is not correct (not a valid GUID or not a valid compound key): return error.
3. Build the Get API Request from the external-name (by either parsing the compound key or using the GUID directly, do not use the spec.forProvider).
   - If resource is not found, that is a drift, not an error case, we therefore return resourceExist: False to trigger a Create() call.
   - Else, updateObservation() and perform needsCreate()/needsUpdate() check as usual. Set the `externalOberservation.Diff` and set the diff as status and event for the resource. This is relevant if the user imports a resource and wants to ensure it doesnt generate any diff (by not setting managment polcy: Update) before having the managed resource fully managed by crossplane.



#### Create()
We know that the resource does not exist so we perform the Create API Request. 
If the request is sucessful, set the external-name with the necessary GUID or compound key from the API response/spec.ForProvider. 
It might be possible that the API works asynchronous and the final external-name is not available after the request. This is a possible szenario but out of scope here.

If the response is `creation failed – resource already exist`, we treat it as an error. We do NOT set the external-name. In the next reconcile Observe(), the resource will be shown as resourceExist: False since the external name is not set and we intend to stay in an error loop. The error documents that the resource could not be created because it already exists. To resume, the user needs to set the external-name properly.

If the request fails for any other reason, we will return the error and dont set the external-name.

#### Update()
Updating non-updateable fields is prevented by using the `kubebuilder:validation:XValidation:rule="self == oldSelf"` annotation to prevent an update to immuteable fields at the API Server. Therefore, we dont need to perform further checks in the provider.

Perform the UPDATE request, return error if it returns error.

If the external-name is a compound key, update the compound key. This is necessary if parts of the compound key were updated. This is unlikely to happen but used as a safeguard in case it happens.

#### Delete()
If our external resource has a field (written into status by Observe)  indicating it is in deletion state, we return.

We use external-name to perform DELETE request.

If the response is 404 – not found (or the equivalent API response for this API), we do NOT treat this as an error. This only happens when the resource was externally removed in the meantime since after Observe(), Delete() is not called if the resource does not exist. Technically we have a drift in here but it can be safely ignored since the drift already covers the desired state.

If our delete request is successful, we return EVEN IF the deletion operation is performed async by the external system (in the next reconcile Observe(), the resourceExist field will be checked by crossplane-runtime and Delete() is called again if the resource still exists). Should the resource still exist in next reconcile, Delete() is triggered again but we check for the deletion state in the external system (= status.observation being something like `DELETING` or so) so we don’t perform another Delete(). If this is not possible, we have to perform another DELETE API call, treat a possible error response as error and return it. Once the resource is deleted in the external system, the Observe() function will determine resourceExist: false and crossplane-runtime removed the MR.

Else, if we have an error in our request, return with error.

### Importing of existing external resources
For importing, setting the right external-name will match the external resource no matter the values in `spec.ForProvider`.

We set the `spec.forProvider` as required, the user can not create managed resources without those values and therefore must also set the required fields. 

Since the matching between MR and external resource is done based on external-name, a wrongly filled spec would result in Update() calls to the resource changing it. To prevent this, the user would not set the managementPolicy to Update. If there is then a missmatch, the Observe() method would determines this, sets the externalObservation.Diff and it would be visible in the debug log. Additionally, we write it into the status field of the resource and write an event to make it visible to the user.

## Open Questions, Things not considered
These might not be important for our APIs if we don’t have this scenarios.

### ID can’t be determined in Create()
Maybe the ID can not be determined in Create() after the API request, maybe it only has a job id/temporal id and later gets a real GUID, the external name needs to be swapped.

### external-name compound key
The delimiter can collide with the field values if they can be in there too. Case sensitive/insensitive matching, leading/trailing spaces. Length limit of annotation, maybe base64 encoded parts of the compound key to avoid delimiter collisions (or escaping of the delimiter character).

Also error case if the compound key matches more than one external resource needs to be handled if a List API is used.

### External-name annotation manually edited
Observe() would return a not found and the resource would be created again with the same `spec.forProvider`. This would probably lead to a creation error. Is this what we want?

### Export tool considerations

The export tool must generate managed resource definitions based on external resource state retrieved through the external system API. To ensure these definitions can be imported successfully, the tool must generate proper `crossplane.io/external-name` annotations.

The export tool **must** be able to derive the `crossplane.io/external-name` annotation value from information obtained through standard API operations (GET, LIST) on external resources.

## Exceptions

Are hybrid resources an exception to this guide as they set their external-name as compound key based on the status fields of the resources.

Upjet resources are an exception as we dont controll how the external-name is defined and set by Upjet.

In general, all exceptions should be noted in the appropriate location: for user in the docs and for developer in the appropriate code location (Observe/Create/Update/Delete functions or API definition).

## References
[1] https://github.com/crossplane/crossplane/blob/main/design/one-pager-managed-resource-api-design.md?plain=1#L151
[2] https://github.com/crossplane/crossplane/issues/1640
[3] https://docs.crossplane.io/latest/guides/import-existing-resources/
[4] https://github.com/SAP/crossplane-provider-cloudfoundry/blob/main/docs/resource-names.md#crossplaneioexternal-name-annotation