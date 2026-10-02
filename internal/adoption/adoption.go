// Package adoption implements opt-in import of existing BTP resources.
//
// A user opts a managed resource in by setting Annotation to "true". While the
// resource has no identifier yet (see Pending), its controller looks it up in
// BTP by its natural key (a name, a subdomain, ...), verifies the match
// against the spec, and adopts it by writing its identifier as the
// external-name. When nothing matches, the controller creates the resource as
// usual.
//
// This is the brownfield "adoption" case the external-name ADR otherwise
// refuses, made explicit: the annotation states the same intent as setting
// crossplane.io/external-name by hand, with the provider finding the
// identifier. It is deliberately separate from package recovery, which runs
// without user intent and therefore only ever re-binds the provider's own
// lost Create() attempts.
//
// Only some kinds support adoption, and some combinations within a kind (such
// as a ServiceBinding with rotation enabled) are refused; each controller
// documents and enforces its own limits.
//
// Where a controller adopts matters. Controllers backed by upjet terraform
// clients (ServiceInstance, ServiceBinding, ServiceManager, CloudManagement)
// must adopt in Connect, before building those clients: upjet seeds its
// in-memory state from the external-name only the first time it sees a
// resource, so a client built while the external-name was still empty would
// observe the adopted resource as missing for the next reconciles, and Create
// would duplicate it. Controllers without such state adopt in Observe.
package adoption

import (
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Annotation opts a managed resource into adoption. Valid values are "true"
// and "false"; anything else is an error.
const Annotation = "btp.sap.crossplane.io/lookup"

// EventReasonAdopted is the reason of the event recorded on adoption.
const EventReasonAdopted event.Reason = "ExternalNameAdopted"

const errPersist = "cannot persist adopted external-name"

// ErrRequeueAfterAdoption is returned by Commit after a successful adoption.
//
// Controllers build their external clients from the external-name in
// Connect(), so the current reconcile must not go on to Create/Update/Delete
// against the empty identity it started with. Returning an error, from
// Connect or Observe, requeues without running any of them and without
// touching the finalizer.
var ErrRequeueAfterAdoption = errors.New("adopted existing BTP resource found by lookup; requeuing to reconcile against it")

func errInvalidValue(value string) error {
	return errors.Errorf("annotation %s must be %q or %q, got %q", Annotation, "true", "false", value)
}

// Pending reports whether obj's controller should look its resource up now:
// obj opted in, has no identifier yet (see identified), and is not being
// deleted. It returns an error when the annotation holds a value other than
// "true" or "false", whether or not a lookup would run.
func Pending(obj metav1.Object) (bool, error) {
	value, ok := obj.GetAnnotations()[Annotation]
	if !ok {
		return false, nil
	}

	switch value {
	case "true":
	case "false":
		return false, nil
	default:
		return false, errInvalidValue(value)
	}

	if identified(obj) {
		return false, nil
	}
	return !meta.WasDeleted(obj), nil
}

// identified reports whether obj's external-name holds a BTP identifier. An
// external-name equal to metadata.name is a default, written by Crossplane's
// initializer or by older provider versions, unless that name is itself a
// GUID: an object deliberately named after its BTP ID is taken at its word.
func identified(obj metav1.Object) bool {
	id := meta.GetExternalName(obj)
	if id == "" {
		return false
	}
	return id != obj.GetName() || uuid.Validate(id) == nil
}

// Commit adopts the BTP resource identified by id: it persists id as mg's
// external-name and records an event naming the lookup key that found it.
// On success it returns ErrRequeueAfterAdoption, which the caller returns
// unchanged. rec may be nil.
//
// Commit also adds the managed reconciler's finalizer. The adopting reconcile
// ends before the managed reconciler would add it, so without it a CR deleted
// in the next few seconds would vanish without the provider deleting the BTP
// resource it has just taken over.
//
// Only the external-name and the finalizer are written, as a patch guarded by
// the object's resourceVersion: Commit must not persist other in-memory
// changes of the reconcile, and must fail rather than overwrite a concurrent
// change, such as an external-name the user set meanwhile.
func Commit(ctx context.Context, kube client.Writer, rec event.Recorder, mg resource.Managed, id, key string) error {
	base, ok := mg.DeepCopyObject().(client.Object)
	if !ok {
		return errors.New(errPersist)
	}
	meta.SetExternalName(mg, id)
	meta.AddFinalizer(mg, managed.FinalizerName)
	patch := client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})
	if err := kube.Patch(ctx, mg, patch); err != nil {
		return errors.Wrap(err, errPersist)
	}

	if rec != nil {
		rec.Event(mg, event.Normal(EventReasonAdopted,
			fmt.Sprintf("Adopted existing BTP resource %s found by lookup (%s)", id, key)))
	}
	return ErrRequeueAfterAdoption
}
