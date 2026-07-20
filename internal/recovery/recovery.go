// Package recovery contains shared helpers for the "orphaned external-name
// recovery" path used by the hand-written BTP resource controllers
// (Subaccount, ServiceInstance, ServiceBinding, ServiceManager,
// CloudManagement).
//
// During Observe(), when the underlying client reports the resource as
// non-existent AND the external-name is a fallback (empty, or == metadata.name),
// the recovery path performs a semantic lookup and, on a unique match that
// passes the ownership check (IsOwnedByCR), patches crossplane.io/external-name
// with the real identifier.
//
// The ownership check keeps this a strict bug-fix (recover our own lost-ID
// Create) rather than a general import mechanism. See #813 for the design
// discussion and docs/contribution-notes/external-name-handling.md for the ADR
// deviation note.
//
// The name "recovery" (not "adoption") is deliberate: the external-name ADR
// uses "adoption" for the brownfield/import case this package refuses.
package recovery

import (
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Kubernetes event reasons emitted by the recovery path.
const (
	EventReasonRecovered         = "ExternalNameRecovered"
	EventReasonLookupFailed      = "RecoveryLookupFailed"
	EventReasonAmbiguous         = "RecoveryAmbiguous"
	EventReasonRefusedBrownfield = "RecoveryRefusedBrownfield"
	EventReasonNoCreateAttempted = "RecoveryRefusedNoCreateAttempted"
)

// ownershipClockSkew absorbs NTP drift between K8s and BTP when comparing
// the CR's create-pending annotation against the BTP resource's created_at.
const ownershipClockSkew = 60 * time.Second

// ownershipUpperWindow bounds how long AFTER our recorded Create attempt the
// BTP resource may have been born and still be treated as ours. Async-
// provisioned resources can have created_at stamped at completion rather than
// at request, hence the generous headroom.
const ownershipUpperWindow = 1 * time.Hour

// IsOwnedByCR reports whether a BTP resource whose server-reported creation
// time is btpCreatedAt could plausibly have been created by our own Create()
// attempt for cr.
//
// The reference is the crossplane.io/external-create-pending annotation
// (written by crossplane-runtime right before every Create, aborting if the
// write fails). A missing annotation means we did not attempt Create — this
// intentionally has NO fallback to metadata.creationTimestamp, which would
// risk pulling in a same-name resource created any time after the CR.
//
// Window: [pending - ownershipClockSkew, pending + ownershipUpperWindow].
func IsOwnedByCR(cr metav1.Object, btpCreatedAt time.Time) bool {
	if btpCreatedAt.IsZero() {
		return false
	}
	pending := meta.GetExternalCreatePending(cr)
	if pending.IsZero() {
		return false
	}
	if btpCreatedAt.Before(pending.Add(-ownershipClockSkew)) {
		return false
	}
	if btpCreatedAt.After(pending.Add(ownershipUpperWindow)) {
		return false
	}
	return true
}

// HasCreateBeenAttempted lets controllers cheaply short-circuit before running
// an expensive semantic lookup that could not possibly pass IsOwnedByCR.
func HasCreateBeenAttempted(cr metav1.Object) bool {
	return !meta.GetExternalCreatePending(cr).IsZero()
}

// ErrRequeueAfterRecovery is returned from Observe() after a successful
// recovery to force a same-turn requeue with backoff.
//
// The controllers capture the external-name at Connect() time, so a same-cycle
// Update()/Delete() would act on the stale fallback. Returning an error
// requeues WITHOUT calling Create/Update/Delete and WITHOUT stripping the
// finalizer. Returning {ResourceExists:false} would be unsafe on the delete
// leg (finalizer strip → orphan).
var ErrRequeueAfterRecovery = errors.New(
	"recovered existing BTP resource by external-name; requeuing to reconcile against it")

// IsFallbackExternalName reports whether externalName is a fallback rather
// than a real BTP identifier (unset, or == metadata.name — Crossplane's
// default initializer value).
//
// A single-UUID external-name (SM/CM phase-1 output) is deliberately NOT
// treated as fallback — re-firing recovery on it used to trap SM/CM in an
// infinite loop where phase-2 never ran.
func IsFallbackExternalName(metadataName, externalName string) bool {
	return externalName == "" || externalName == metadataName
}
