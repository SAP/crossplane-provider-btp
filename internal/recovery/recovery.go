// Package recovery contains shared helpers for the "orphaned external-name
// recovery" path used by the hand-written BTP resource controllers
// (Subaccount, ServiceInstance, ServiceBinding, ServiceManager,
// CloudManagement).
//
// # What it does
//
// During Observe(), when the underlying client reports the resource as
// non-existent AND the external-name is a fallback (empty, or == metadata.name
// — Crossplane's default initializer value), the recovery path performs a
// semantic lookup against the BTP APIs and, on a unique match that also
// passes the ownership check (IsOwnedByCR), patches
// crossplane.io/external-name with the real identifier.
//
// The ownership check is what keeps this a strict bug-fix (recovers our own
// lost-ID Create) rather than a general import mechanism: recovery only
// proceeds when the BTP resource was plausibly created by our own Create()
// attempt for this CR — see IsOwnedByCR.
//
// # Why "recovery" and not "adoption"
//
// The external-name ADR uses the word "adoption" for the brownfield/import
// case (an existing BTP resource is imported into Crossplane management via
// an explicit crossplane.io/external-name annotation on a new CR). That case
// is EXACTLY what this package refuses. To avoid terminology overlap, this
// package is called `recovery`: it recovers CRs from their own lost-ID
// Create rather than adopting foreign resources.
//
// # Which failure modes are actually reachable
//
// crossplane-runtime writes crossplane.io/external-create-pending BEFORE every
// Create and only writes external-create-succeeded on success. If those two
// annotations disagree and the resource's external-name is non-deterministic
// (as all BTP identifiers are — random GUIDs), the reconciler refuses to run
// Observe at all and freezes the resource with a "cannot determine creation
// result" error. So a pod crash between Create and the status write is NOT
// reachable by this recovery — the reconciler stops before Observe.
//
// The reachable states are narrower than that:
//
//   - ServiceManager / CloudManagement two-phase create. The operator writes
//     the instance's UUID as external-name after phase-1 (createInstance) and
//     upgrades to the compound "<sID>/<bID>" only after phase-2
//     (createBinding). If phase-2's Create is interrupted, the CR ends up
//     with a single-UUID external-name (which is NOT a fallback per
//     IsFallbackExternalName, so recovery doesn't fire — the two-phase loop
//     resumes on its own). But if the operator was torn down and recreated
//     between the two phases, or an early failure prevented ever persisting
//     phase-1's UUID, the CR can end up with a fallback external-name while
//     phase-1 already exists in BTP.
//
//   - Any hand-written controller path that persists the CR's status/spec
//     between BTP Create and the external-name update, and crashes in
//     between (rare, but has been observed on ServiceInstance/ServiceBinding
//     in the wild — the BTP resource exists but the CR has no ID).
//
// In every reachable case the pending annotation was written before the
// Create that succeeded, so IsOwnedByCR can rely on it as the reference time.
// See #813 for the design discussion.
package recovery

import (
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Kubernetes event reasons emitted by the recovery path.
const (
	// EventReasonRecovered is emitted (Normal) once per successful recovery.
	EventReasonRecovered = "ExternalNameRecovered"
	// EventReasonLookupFailed is emitted (Warning) when the semantic lookup
	// call itself fails (network/auth/5xx). The reconcile falls through to the
	// pre-recovery result and retries on the next cycle.
	EventReasonLookupFailed = "RecoveryLookupFailed"
	// EventReasonAmbiguous is emitted (Warning) when the semantic lookup finds
	// more than one candidate and refuses to guess.
	EventReasonAmbiguous = "RecoveryAmbiguous"
	// EventReasonRefusedBrownfield is emitted (Warning) when the semantic
	// lookup finds exactly one candidate but its BTP created_at is outside
	// the window where our own Create() attempt could have produced it —
	// so it is a brownfield resource that the user must adopt explicitly by
	// setting crossplane.io/external-name (per the external-name ADR). See
	// IsOwnedByCR.
	EventReasonRefusedBrownfield = "RecoveryRefusedBrownfield"
	// EventReasonNoCreateAttempted is emitted (Warning) when a semantic-lookup
	// match exists but the CR has no crossplane.io/external-create-pending
	// annotation, so we have no evidence this controller ever attempted
	// Create() for this CR. Refuses to recover, defaulting to safety.
	EventReasonNoCreateAttempted = "RecoveryRefusedNoCreateAttempted"
)

// ownershipClockSkew absorbs NTP drift between the K8s API server and BTP
// when comparing the CR's create-pending annotation against the BTP
// resource's server-reported created_at. See IsOwnedByCR.
//
// One minute is conservative: even a poorly-NTPed shoot with a few seconds
// of drift comfortably passes.
const ownershipClockSkew = 60 * time.Second

// ownershipUpperWindow bounds how long AFTER our recorded Create attempt the
// BTP resource may have been born and still be treated as ours. The reference
// (external-create-pending) is stamped right before our Create call, but BTP's
// server-reported created_at can lag that call for asynchronously-provisioned
// resources whose created_at is stamped at completion rather than at request.
// One hour is generously above any observed provisioning lag while still
// refusing a same-key resource that appeared much later (which cannot be the
// result of our attempt).
const ownershipUpperWindow = 1 * time.Hour

// IsOwnedByCR reports whether a BTP resource whose server-reported creation
// time is btpCreatedAt could plausibly have been created by our own Create()
// attempt for cr.
//
// The check uses the crossplane.io/external-create-pending annotation as the
// reference — crossplane-runtime writes it right before every Create, and
// aborts if the write fails. So a non-zero pending timestamp is a reliable
// signal that we attempted Create; a missing one means we did not.
//
// When present, the ownership window is symmetric around the attempt time:
//
//	[pending - ownershipClockSkew, pending + ownershipUpperWindow]
//
// Anything born outside that window cannot be the result of our attempt and
// is refused as brownfield. This intentionally has NO fallback to the CR's
// creationTimestamp: without evidence that we attempted Create, we have no
// business recovering an existing BTP resource by name — a same-name
// resource that shows up later would be pulled in.
//
// A zero btpCreatedAt (server didn't populate the field) is treated as
// unknown-provenance and refused, defaulting to safety.
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

// HasCreateBeenAttempted reports whether crossplane-runtime has ever stamped
// the crossplane.io/external-create-pending annotation on cr, i.e. whether we
// have any evidence this controller attempted Create() for this CR. The
// recovery path uses this to short-circuit with a clear reason before running
// an expensive semantic lookup that could not possibly pass IsOwnedByCR.
func HasCreateBeenAttempted(cr metav1.Object) bool {
	return !meta.GetExternalCreatePending(cr).IsZero()
}

// ErrRequeueAfterRecovery is returned from Observe() after a successful
// recovery.
//
// All four hand-written controllers capture the external-name at Connect()
// time (SI via TfProxyConnector→TfResource; SB via CreateClient; SM/CM via
// their tf-client sub-resource builders). A same-cycle Update()/Delete() would
// therefore still act on the stale fallback name. Returning an error forces
// Crossplane to requeue with backoff WITHOUT calling Create/Update/Delete and
// WITHOUT stripping the finalizer (errors short-circuit before finalizer
// handling). On the next reconcile, Connect() rebuilds the external client from
// the freshly persisted external-name and Observe() sees the real state.
//
// Returning {ResourceExists:false} instead would be unsafe on the delete leg:
// it would strip the finalizer and orphan the BTP resource.
var ErrRequeueAfterRecovery = errors.New(
	"recovered existing BTP resource by external-name; requeuing to reconcile against it")

// IsFallbackExternalName reports whether externalName is a fallback rather than
// a real BTP identifier: either unset, or equal to the resource's metadata.name
// (the value Crossplane's default initializer stamps when the annotation was
// never set). Per the external-name ADR, a fallback expresses no user intent to
// adopt a specific resource, so it is safe to overwrite via semantic lookup.
//
// This is the ONLY trigger used by the recovery path, for every kind
// (Subaccount, ServiceInstance, ServiceBinding, ServiceManager,
// CloudManagement). Earlier drafts also treated any non-compound external-name
// (e.g. a single UUID for the SM/CM "<sID>/<bID>" scheme) as needing
// recovery — that was a bug: after the operator's phase-1 Create writes
// the instance's UUID as external-name, the same reconcile loop is still
// mid-two-phase-creation and just needs to run phase-2 (create the binding).
// Re-firing recovery in that state trapped SM/CM in an infinite loop where
// phase-2 never ran and the binding was never created. See the commit that
// removed IsFallbackOrNonCompound for the reproducer.
func IsFallbackExternalName(metadataName, externalName string) bool {
	return externalName == "" || externalName == metadataName
}
