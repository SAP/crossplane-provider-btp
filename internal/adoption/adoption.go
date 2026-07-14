// Package adoption contains shared helpers for the "orphaned external-name
// adoption" heal path used by the hand-written BTP resource controllers
// (ServiceInstance, ServiceBinding, ServiceManager, CloudManagement).
//
// Background: under certain async-create failure modes (pod restart mid-create,
// two reconcilers racing a create, or a ServiceManager being torn down and
// recreated) a managed resource can end up with only a fallback external-name
// (empty, or equal to metadata.name — Crossplane's default initializer value)
// even though BTP successfully created the corresponding resource. From then on
// the CR is orphaned from its external counterpart: Observe/Update can never
// resolve it, and — worst of all — Delete tries to delete a non-existent ID,
// gets a 404, treats it as "already gone", strips the finalizer and leaves the
// BTP resource orphaned.
//
// The heal path (gated behind the BTP_ADOPT_ORPHANED_EXTERNAL_NAMES provider
// flag) runs during Observe(): when the underlying client reports the resource
// as non-existent and the external-name is a fallback, it performs a semantic
// lookup against the BTP Service Manager API and, on a unique match, adopts the
// resource by patching crossplane.io/external-name with the real identifier.
package adoption

import (
	"time"

	"github.com/pkg/errors"
)

// Kubernetes event reasons emitted by the heal path.
const (
	// EventReasonAdopted is emitted (Normal) once per successful adoption.
	EventReasonAdopted = "ExternalNameAdopted"
	// EventReasonLookupFailed is emitted (Warning) when the semantic lookup
	// call itself fails (network/auth/5xx). The reconcile falls through to the
	// pre-heal result and retries on the next cycle.
	EventReasonLookupFailed = "AdoptionLookupFailed"
	// EventReasonAmbiguous is emitted (Warning) when the semantic lookup finds
	// more than one candidate and refuses to guess.
	EventReasonAmbiguous = "AdoptionAmbiguous"
	// EventReasonRefusedBrownfield is emitted (Warning) when the semantic
	// lookup finds exactly one candidate but its BTP created_at predates the
	// CR's own creationTimestamp — the provider could not have created it,
	// so it is a brownfield resource that the user must adopt explicitly by
	// setting crossplane.io/external-name. See adoption.IsOwnedByCR.
	EventReasonRefusedBrownfield = "AdoptionRefusedBrownfield"
)

// ownershipClockSkew is the tolerance applied to the (btpCreatedAt >=
// crCreatedAt) ownership check to handle clock differences between the K8s
// API server and the BTP API servers.
//
// One minute is generous but very conservative: even a poorly-NTPed shoot
// with a couple of seconds of drift comfortably passes, while still refusing
// any resource that predates the CR by more than a minute (which cannot be
// the result of our own Create call — that gets kicked off strictly after
// the CR appears, and BTP Creates are effectively instant on the server
// side). The trade-off: a brownfield resource created within 60s of our CR
// could still be adopted, but that requires someone else provisioning a BTP
// resource with the same semantic key inside a 60-second window — an
// implausible race in practice.
const ownershipClockSkew = 60 * time.Second

// IsOwnedByCR reports whether a BTP resource whose server-reported creation
// time is btpCreatedAt could plausibly have been created by the CR whose K8s
// creationTimestamp is crCreatedAt.
//
// The invariant this enforces is: the CR ALWAYS exists before Create() is
// called, so any BTP resource we created ourselves must have been born at or
// after our CR. If btpCreatedAt is meaningfully earlier than crCreatedAt, the
// resource cannot be ours; it is a brownfield resource that the user must
// adopt explicitly (per the external-name ADR).
//
// A zero btpCreatedAt (server didn't populate the field) is treated as
// unknown-provenance and refused, defaulting to safety.
//
// This is what keeps the heal path a strict bug-fix (recovers our own
// lost-ID create) rather than a general import mechanism.
func IsOwnedByCR(crCreatedAt, btpCreatedAt time.Time) bool {
	if btpCreatedAt.IsZero() {
		return false
	}
	return !btpCreatedAt.Before(crCreatedAt.Add(-ownershipClockSkew))
}

// ErrRequeueAfterAdopt is returned from Observe() after a successful adoption.
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
var ErrRequeueAfterAdopt = errors.New(
	"adopted existing BTP resource by external-name; requeuing to reconcile against it")

// IsFallbackExternalName reports whether externalName is a fallback rather than
// a real BTP identifier: either unset, or equal to the resource's metadata.name
// (the value Crossplane's default initializer stamps when the annotation was
// never set). Per the external-name ADR, a fallback expresses no user intent to
// adopt a specific resource, so it is safe to overwrite via semantic lookup.
//
// This is the ONLY trigger used by the adoption heal, for every kind
// (ServiceInstance, ServiceBinding, ServiceManager, CloudManagement,
// Subaccount). Earlier drafts also treated any non-compound external-name
// (e.g. a single UUID for the SM/CM "<sID>/<bID>" scheme) as needing
// adoption — that was a bug: after the operator's phase-1 Create writes
// the instance's UUID as external-name, the same reconcile loop is still
// mid-two-phase-creation and just needs to run phase-2 (create the binding).
// Re-firing adoption in that state trapped SM/CM in an infinite adoption
// loop where phase-2 never ran and the binding was never created. See the
// commit that removed IsFallbackOrNonCompound for the reproducer.
func IsFallbackExternalName(metadataName, externalName string) bool {
	return externalName == "" || externalName == metadataName
}
