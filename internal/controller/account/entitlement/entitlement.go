package entitlement

import (
	"context"
	"fmt"
	"reflect"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	entitlementclient "github.com/sap/crossplane-provider-btp/internal/clients/entitlement"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/mrstatus"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotEntitlement      = "managed resource is not a Entitlement custom resource"
	errConnect             = "while connecting to provider"
	errObserve             = "while observing entitlement"
	errUpdateObservation   = "while updating observation"
	errBuildExternalName   = "cannot build external-name from spec"
	errParseExternalName   = "cannot parse external-name"
	errUpdateExternalName  = "cannot update external-name to compound key"
	errResolveIdentity     = "while resolving entitlement identity"
	errDescribeInstance    = "while describing instance"
	errFindRelated         = "while finding related entitlements"
	errMergeRelated        = "while merging related entitlements"
	errGenerateObservation = "while generating observation"
	errCreate              = "while creating entitlement"
	errCreateInstance      = "while creating instance"
	errUpdate              = "while updating entitlement"
	errUpdateInstance      = "while updating instance"
	errDelete              = "while deleting entitlement"
	errDeleteInstance      = "while deleting instance"
	errListEntitlements    = "while listing entitlements"
)

// reasonAutoAssignedPreserved is the Kubernetes event reason emitted when a
// deleting AutoAssigned entitlement is finalized without touching BTP.
const reasonAutoAssignedPreserved = "AutoAssignedPreserved"

var (
	noOpFilter = func(entitlement apisv1alpha1.Entitlement) bool {
		return true
	}
	// errExistingAssignmentRequiresAdoption is returned by Observe and
	// Create when BTP already has a genuine assignment that no sibling
	// (see mayAdopt) has proven this CR's aggregate owns.
	errExistingAssignmentRequiresAdoption = errors.New(
		"assignment already exists. Please set crossplane.io/external-name annotation to adopt the existing resource",
	)
	// errUnownedAssignmentBlocksFinalize is resolveUnjoinedDeletion's
	// refusal when a deleting CR never proved ownership of a
	// still-reserved assignment (or lost its identity stamp before persisting it).
	errUnownedAssignmentBlocksFinalize = errors.New(
		"an unowned assignment with reserved quota still matches this entitlement's identity, so deletion cannot finalize. " +
			"If this assignment is genuinely unrelated and should be left alone, remove this resource's finalizer to delete it without modifying BTP. " +
			"If this resource did create the assignment (for example Create succeeded against BTP but a later write of the critical crossplane.io/external-name annotation failed), " +
			"set crossplane.io/external-name to the compound key first so deletion actually removes the assignment instead of stranding it.",
	)
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	usage           providerconfig.LegacyTracker
	resourcetracker tracking.ReferenceResolverTracker
	newServiceFn    func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)
	recorder        event.Recorder
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	_, ok := mg.(*apisv1alpha1.Entitlement)
	if !ok {
		return nil, errors.New(errNotEntitlement)
	}

	btpclient, err := providerconfig.CreateClient(ctx, mg, c.kube, c.usage, c.newServiceFn, c.resourcetracker)
	if err != nil {
		return nil, errors.Wrap(err, errConnect)
	}
	return &external{
		kube:     c.kube,
		client:   entitlementclient.NewEntitlementsClient(*btpclient),
		tracker:  c.resourcetracker,
		recorder: c.recorder,
	}, nil
}

// Disconnect is a no-op for the external client to close its connection.
// Since we dont need this, we only have it to fullfil the interface.
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

// emit records a Kubernetes event when a recorder is configured.
func (c *external) emit(cr resource.Managed, ev event.Event) {
	if c.recorder != nil {
		c.recorder.Event(cr, ev)
	}
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	// A 'client' used to connect to the external resource API. In practice this
	// would be something like an AWS SDK client.
	kube    client.Client
	client  entitlementclient.Client
	tracker tracking.ReferenceResolverTracker
	// recorder emits Kubernetes events for the deletion carve-out path and
	// for aggregate drift detection (see Observe's Drift block). May be nil.
	recorder event.Recorder
}

// externalNameState distinguishes the three shapes keyForObserve resolves the external-name annotation into.
type externalNameState int

const (
	// externalNameEmpty means the annotation is unset.
	externalNameEmpty externalNameState = iota
	// externalNameLegacy means the annotation still holds the pre-ADR default sentinel (metadata.name).
	externalNameLegacy
	// externalNameCurrent means the annotation holds a parsed, spec-agreeing compound key.
	externalNameCurrent
)

// parseCurrentExternalName parses value as a compound external-name
// key and rejects a result that disagrees with cr's current (immutable)
// spec identity; used by keyForObserve and currentExternalNameKey.
func parseCurrentExternalName(cr *apisv1alpha1.Entitlement, value string) (entitlementclient.ExternalNameKey, error) {
	key, err := entitlementclient.ParseExternalName(value)
	if err != nil {
		return entitlementclient.ExternalNameKey{}, errors.Wrap(err, errParseExternalName)
	}
	if mismatch := key.Mismatch(cr); mismatch != "" {
		return entitlementclient.ExternalNameKey{},
			errors.Wrap(entitlementclient.ErrExternalNameSpecMismatch, mismatch)
	}
	return key, nil
}

// keyForObserve resolves cr's identity from its external-name
// annotation. An empty or legacy annotation builds the key from spec;
// any other value must parse as a compound key that agrees with spec.
func keyForObserve(cr *apisv1alpha1.Entitlement) (entitlementclient.ExternalNameKey, externalNameState, error) {
	value := meta.GetExternalName(cr)
	if value == "" || value == cr.Name {
		state := externalNameEmpty
		if value == cr.Name {
			state = externalNameLegacy
		}
		key, err := entitlementclient.NewExternalNameKey(cr)
		if err != nil {
			return entitlementclient.ExternalNameKey{}, state, errors.Wrap(err, errBuildExternalName)
		}
		return key, state, nil
	}
	key, err := parseCurrentExternalName(cr, value)
	if err != nil {
		return entitlementclient.ExternalNameKey{}, externalNameCurrent, err
	}
	return key, externalNameCurrent, nil
}

// deletingWithReservedQuota reports whether a deleting cr's
// assignFailedNoQuota shape still reserves quota via
// UnlimitedAmountAssigned. Contrast assignmentStillReserved, which
// answers the more general question outside that shape. nil-safe.
func deletingWithReservedQuota(cr *apisv1alpha1.Entitlement) bool {
	if cr.GetDeletionTimestamp() == nil {
		return false
	}
	return cr.Status.AtProvider != nil && cr.Status.AtProvider.Assigned != nil &&
		cr.Status.AtProvider.Assigned.UnlimitedAmountAssigned
}

// deletingAutoAssigned reports whether a deleting cr's BTP assignment is
// AutoAssigned; it must finalize without writing to BTP, only emitting reasonAutoAssignedPreserved. nil-safe.
func deletingAutoAssigned(cr *apisv1alpha1.Entitlement) bool {
	return cr.GetDeletionTimestamp() != nil &&
		cr.Status.AtProvider != nil &&
		cr.Status.AtProvider.Assigned != nil &&
		cr.Status.AtProvider.Assigned.AutoAssigned
}

// adoptionGuardApplies reports whether the aggregate-ownership adoption
// guard applies: a genuinely empty annotation against a real,
// non-AutoAssigned, reserved assignment (including via deletingWithReservedQuota) needs sibling proof (mayAdopt).
func adoptionGuardApplies(
	cr *apisv1alpha1.Entitlement,
	state externalNameState,
	instance *entitlementclient.Instance,
) bool {
	if state != externalNameEmpty || instance.Assignment == nil {
		return false
	}
	if cr.Status.AtProvider != nil && cr.Status.AtProvider.Assigned != nil && cr.Status.AtProvider.Assigned.AutoAssigned {
		return false
	}
	if !assignFailedNoQuota(cr) {
		return true
	}
	return deletingWithReservedQuota(cr)
}

// observeExternalName resolves cr's identity and BTP assignment for
// Observe. A non-nil *managed.ExternalObservation is terminal; nil/nil
// means Observe's shared needsCreate/needsUpdate/deletion logic decides.
func (c *external) observeExternalName(ctx context.Context, cr *apisv1alpha1.Entitlement) (*managed.ExternalObservation, error) {
	key, state, err := keyForObserve(cr)
	if err != nil {
		return nil, err
	}

	instance, err := c.client.DescribeInstance(ctx, key)
	if err != nil {
		return nil, errors.Wrap(err, errDescribeInstance)
	}

	if state == externalNameEmpty || state == externalNameLegacy {
		if instance.Assignment == nil {
			return &managed.ExternalObservation{ResourceExists: false}, nil
		}
		if err := c.updateObservationFrom(ctx, cr, instance); err != nil {
			return nil, err
		}

		// A PROCESSING_FAILED report with nothing reserved mirrors
		// needsCreate's "not created yet" reading. Excludes a deleting CR
		// with quota still reserved (deletingWithReservedQuota) or an
		// AutoAssigned one (falls through to the carve-out below instead).
		if assignFailedNoQuota(cr) && !deletingWithReservedQuota(cr) && !deletingAutoAssigned(cr) {
			return &managed.ExternalObservation{ResourceExists: false}, nil
		}

		if adoptionGuardApplies(cr, state, instance) {
			// A typo'd or coincidental spec match must not silently adopt
			// an unrelated BTP assignment: require sibling ownership proof.
			adopted, err := c.mayAdopt(ctx, cr, key)
			if err != nil {
				return nil, err
			}
			if !adopted {
				if cr.GetDeletionTimestamp() != nil {
					return c.resolveUnjoinedDeletion(ctx, cr)
				}
				return nil, errExistingAssignmentRequiresAdoption
			}
		}

		// Legacy or self-adopted AutoAssigned identity is persisted for
		// later reconciles; skipped while deleting since finalize happens first.
		if cr.GetDeletionTimestamp() == nil {
			if err := c.persistExternalName(ctx, cr, key); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}

	if err := c.updateObservationFrom(ctx, cr, instance); err != nil {
		return nil, err
	}
	return nil, nil
}

// assignmentStillReserved reports whether cr's currently observed BTP
// assignment still reserves something, for resolveUnjoinedDeletion's
// zero-remaining-sibling branch. UnlimitedAmountAssigned is the
// authoritative, independent signal for an enable-based assignment;
// otherwise a present, positive Amount is reserved, and nil counts as reserved.
func assignmentStillReserved(cr *apisv1alpha1.Entitlement) bool {
	if cr.Status.AtProvider == nil || cr.Status.AtProvider.Assigned == nil {
		return true
	}
	if cr.Status.AtProvider.Assigned.UnlimitedAmountAssigned {
		return true
	}
	amount := cr.Status.AtProvider.Assigned.Amount
	return amount == nil || *amount > 0
}

// resolveUnjoinedDeletion decides observeExternalName's empty-name
// deletion shortcut once mayAdopt finds no sibling proof of ownership.
// Neither outcome may write to BTP: zero remaining siblings finalize
// only if assignmentStillReserved reports nothing reserved, else refuse
// via errUnownedAssignmentBlocksFinalize; with siblings, finalize when
// deletionCompleteForSiblings reports their need covers BTP's
// assignment, which that helper's zero-item shortcut cannot tell. A
// proven aggregate never reaches here: mayAdopt lets cr join instead, and
// Observe's own deletion path issues any reduction.
func (c *external) resolveUnjoinedDeletion(ctx context.Context, cr *apisv1alpha1.Entitlement) (*managed.ExternalObservation, error) {
	siblings, err := c.findRelatedEntitlements(
		ctx, cr,
		func(candidate apisv1alpha1.Entitlement) bool { return candidate.UID != cr.UID },
	)
	if err != nil {
		return nil, errors.Wrap(err, errFindRelated)
	}
	if len(siblings.Items) == 0 {
		if !assignmentStillReserved(cr) {
			return &managed.ExternalObservation{ResourceExists: false}, nil
		}
		return nil, errUnownedAssignmentBlocksFinalize
	}
	complete, err := deletionCompleteForSiblings(cr, siblings)
	if err != nil {
		return nil, errors.Wrap(err, errMergeRelated)
	}
	if complete {
		return &managed.ExternalObservation{ResourceExists: false}, nil
	}
	return nil, errUnownedAssignmentBlocksFinalize
}

// siblingProvesOwnership reports whether sibling's own external-name
// annotation already proves key (or the pre-ADR legacy sentinel) is provider-managed.
func siblingProvesOwnership(sibling *apisv1alpha1.Entitlement, key entitlementclient.ExternalNameKey) bool {
	value := meta.GetExternalName(sibling)
	return value == key.String() || value == sibling.Name
}

// mayAdopt reports whether a non-deleting same-key sibling other than
// cr already proves key is provider-managed (qualifier-aware, via findRelatedEntitlements).
func (c *external) mayAdopt(
	ctx context.Context,
	cr *apisv1alpha1.Entitlement,
	key entitlementclient.ExternalNameKey,
) (bool, error) {
	siblings, err := c.findRelatedEntitlements(
		ctx,
		cr,
		func(candidate apisv1alpha1.Entitlement) bool {
			return candidate.UID != cr.UID
		},
	)
	if err != nil {
		return false, errors.Wrap(err, errFindRelated)
	}
	for i := range siblings.Items {
		if siblingProvesOwnership(&siblings.Items[i], key) {
			return true, nil
		}
	}
	return false, nil
}

// persistExternalName stamps cr's external-name annotation with key's
// compound form and persists it via kube.Update; only Observe calls this.
func (c *external) persistExternalName(
	ctx context.Context,
	cr *apisv1alpha1.Entitlement,
	key entitlementclient.ExternalNameKey,
) error {
	meta.SetExternalName(cr, key.String())
	// The CRD has a status subresource, so Update returns the stored status and
	// controller-runtime decodes it over our fresh observation. DeepCopy (not a
	// pointer copy - decode mutates the existing pointee in place) preserves it.
	observation := cr.Status.AtProvider.DeepCopy()
	err := c.kube.Update(ctx, cr)
	cr.Status.AtProvider = observation
	return errors.Wrap(err, errUpdateExternalName)
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*apisv1alpha1.Entitlement)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotEntitlement)
	}

	obs, err := c.observeExternalName(ctx, cr)
	cr.SetConditions(c.softValidation(cr))
	c.tracker.SetConditions(ctx, cr)
	if err != nil {
		if errors.Is(err, errExistingAssignmentRequiresAdoption) || errors.Is(err, errUnownedAssignmentBlocksFinalize) {
			// Both errors are terminal and user-actionable; wrapping them
			// in errUpdateObservation would bury the remediation text.
			return managed.ExternalObservation{}, err
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errUpdateObservation)
	}
	if obs != nil {
		return *obs, nil
	}

	// upstream issue #280: a rejected assignment must never look healthy.
	//
	// This lands on the resource for every path that leaves Observe reporting
	// the external resource as existing. It does NOT survive the needsCreate
	// path (amount == 0): there the managed reconciler goes on to mark
	// Creating(), which replaces the Ready condition before the status is
	// persisted. That path is a retry of the assignment, and the rejection
	// reason resurfaces here as soon as BTP reports PROCESSING_FAILED again on
	// an assignment that reserved a non-zero amount.
	if entitlementProcessingFailed(cr) {
		cr.Status.SetConditions(entitlementFailedCondition(cr))
	}

	// Needs create?
	//
	// needsCreate alone would misreport a deleting CR whose
	// still-reserved assignFailedNoQuota-shape assignment (see
	// deletingWithReservedQuota) as absent; this exclusion routes it to
	// the deletion carve-outs below instead.
	if c.needsCreate(cr) && !deletingWithReservedQuota(cr) {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	if deletingAutoAssigned(cr) {
		c.emit(cr, event.Normal(reasonAutoAssignedPreserved,
			"BTP auto-assigned entitlement remains available and was not modified"))
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// When deleting, sibling CRs continue managing any remaining amount.
	if cr.GetDeletionTimestamp() != nil {
		deleted, err := c.deletionComplete(ctx, cr)
		if err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errFindRelated)
		}
		if deleted {
			return managed.ExternalObservation{
				ResourceExists: false,
			}, nil
		}
		// BTP not yet updated — let Delete() handle it
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: true,
		}, nil
	}

	// By this point Assigned is known present and cr is known not
	// deleting, the two preconditions the ADR requires before drift is
	// computed, and before needsUpdate's own early return below.
	previousDrift := cr.Status.GetCondition(apisv1alpha1.DriftConditionType).Message
	diff := calculateDiff(cr)
	if diff == "" {
		cr.Status.SetConditions(apisv1alpha1.NoDrift())
	} else {
		cr.Status.SetConditions(apisv1alpha1.DriftDetected(diff))
		// Dedupe against the message PERSISTED by the previous reconcile
		// (read above, before this call overwrote it), so an unchanged
		// diff does not emit an identical Warning event on every poll.
		if diff != previousDrift {
			c.emit(cr, event.Warning(event.Reason(apisv1alpha1.DriftDetectedReason), errors.New(diff)))
		}
	}

	// Needs Update?
	if c.needsUpdate(cr) {
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: false,
			Diff:             diff,
		}, nil
	}
	switch cr.Status.AtProvider.Assigned.EntityState { //nolint:exhaustive
	case apisv1alpha1.EntitlementStatusOk:
		cr.Status.SetConditions(xpv1.Available())
	// PROCESSING_FAILED means BTP rejected or failed the last operation on
	// this entitlement. It never reports Available, whatever amount is
	// currently assigned, and the platform's own stateMessage is copied into
	// the condition so the rejection reason is readable on the resource.
	//
	// This branch previously reported Available whenever an amount was still
	// assigned, on the grounds that a still-in-use entitlement should not flap
	// orchestration that depends on it. That trade is rejected: reporting a
	// healthy state for an operation the platform refused is what let a whole
	// class of failures look healthy at every observable layer, with the
	// rejection message dropped on the floor. Dependents are better served by
	// an honest NotReady they can act on.
	//
	// Only the condition changes here. assignFailedNoQuota() and needsCreate()
	// keep their current semantics, so the assign-time failure with nothing
	// reserved (amount == 0 / nil) still retries via Create.
	case apisv1alpha1.EntitlementStatusProcessingFailed:
		cr.Status.SetConditions(entitlementFailedCondition(cr))
	case apisv1alpha1.EntitlementStatusProcessing:
		cr.Status.SetConditions(xpv1.Creating())
	case apisv1alpha1.EntitlementStatusStarted:
		cr.Status.SetConditions(xpv1.Creating())
	default:
		cr.Status.SetConditions(xpv1.Unavailable())
	}

	return managed.ExternalObservation{
		// Return false when the external resource does not exist. This lets
		// the managed resource reconciler know that it needs to call Create to
		// (re)create the resource, or that it has successfully been deleted.
		ResourceExists: true,

		// Return false when the external resource exists, but it not up to date
		// with the desired managed resource state. This lets the managed
		// resource reconciler know that it needs to call Update.
		ResourceUpToDate: true,

		// Diff surfaces calculateDiff's comparison for logging;
		// status.conditions[Drift] and the Warning event above are the
		// actually persisted signal.
		Diff: diff,
	}, nil
}

// calculateDiff reports drift between the aggregate desired state
// (status.atProvider.required, not cr's own spec.forProvider) and what
// BTP reports. It is enable-based, governed by the enable comparison,
// whenever Required.Enable is non-nil while Required.Amount is nil.
func calculateDiff(cr *apisv1alpha1.Entitlement) string {
	if cr.Status.AtProvider == nil || cr.Status.AtProvider.Assigned == nil {
		return ""
	}
	assigned := cr.Status.AtProvider.Assigned
	var required apisv1alpha1.EntitlementSummary
	if cr.Status.AtProvider.Required != nil {
		required = *cr.Status.AtProvider.Required
	}

	switch {
	case required.Amount != nil || (required.Enable == nil && assigned.Amount != nil):
		if reflect.DeepEqual(required.Amount, assigned.Amount) {
			return ""
		}
		return fmt.Sprintf("amount mismatch (desired=%s, observed=%s)",
			formatAmountForDiff(required.Amount), formatAmountForDiff(assigned.Amount))
	case required.Enable == nil || *required.Enable != assigned.UnlimitedAmountAssigned:
		return fmt.Sprintf("enable mismatch (desired=%s, observed=%t)",
			formatEnableForDiff(required.Enable), assigned.UnlimitedAmountAssigned)
	default:
		return ""
	}
}

// formatAmountForDiff renders an amount for calculateDiff's message: the
// integer when present, or "<unset>" when nil.
func formatAmountForDiff(amount *int) string {
	if amount == nil {
		return "<unset>"
	}
	return fmt.Sprintf("%d", *amount)
}

// formatEnableForDiff renders a desired enable flag for calculateDiff's
// message: "true"/"false" when present, or "<unset>" when nil.
func formatEnableForDiff(enable *bool) string {
	if enable == nil {
		return "<unset>"
	}
	return fmt.Sprintf("%t", *enable)
}

// updateObservationFrom populates cr.Status.AtProvider from an
// already-fetched instance, avoiding a second DescribeInstance call.
func (c *external) updateObservationFrom(
	ctx context.Context,
	cr *apisv1alpha1.Entitlement,
	instance *entitlementclient.Instance,
) error {
	entitlements, err := c.findRelatedEntitlements(ctx, cr, noOpFilter)
	if err != nil {
		return errors.Wrap(err, errFindRelated)
	}
	cr.Status.AtProvider, err = entitlementclient.GenerateObservation(instance, entitlements)
	if err != nil {
		return errors.Wrap(err, errGenerateObservation)
	}
	return nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*apisv1alpha1.Entitlement)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotEntitlement)
	}

	key, state, err := keyForObserve(cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errResolveIdentity)
	}

	// Observe's decision to call Create may rely on a TTL-cached read;
	// re-read bypassing that cache and re-run the ownership guard here,
	// since another actor may have created the assignment since.
	instance, err := c.client.DescribeInstanceFresh(ctx, key)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errDescribeInstance)
	}
	if err := c.updateObservationFrom(ctx, cr, instance); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errUpdateObservation)
	}

	autoAssigned := instance.Assignment != nil &&
		cr.Status.AtProvider != nil && cr.Status.AtProvider.Assigned != nil &&
		cr.Status.AtProvider.Assigned.AutoAssigned

	// adoptionGuardApplies only applies to a genuinely empty annotation
	// against a real, reserved, non-AutoAssigned assignment (see its doc comment).
	if adoptionGuardApplies(cr, state, instance) {
		adopted, err := c.mayAdopt(ctx, cr, key)
		if err != nil {
			return managed.ExternalCreation{}, err
		}
		if !adopted {
			return managed.ExternalCreation{}, errExistingAssignmentRequiresAdoption
		}
	}

	if !autoAssigned {
		if err := c.client.CreateInstance(ctx, key, cr); err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, errCreateInstance)
		}
	}

	// Identity is stamped only once the write succeeds or is safely
	// skipped as an AutoAssigned adoption; a current annotation is left untouched.
	if state == externalNameEmpty || state == externalNameLegacy {
		meta.SetExternalName(cr, key.String())
	}

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

// currentExternalNameKey resolves cr's identity for Update alone;
// unlike keyForObserve it never falls back to spec, since Observe
// already persisted a compound key by Update time.
func currentExternalNameKey(cr *apisv1alpha1.Entitlement) (entitlementclient.ExternalNameKey, error) {
	return parseCurrentExternalName(cr, meta.GetExternalName(cr))
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*apisv1alpha1.Entitlement)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotEntitlement)
	}

	if cr.Status.AtProvider == nil {
		return managed.ExternalUpdate{}, nil
	}

	if c.updateInProgress(cr) {
		return managed.ExternalUpdate{}, nil
	}

	key, err := currentExternalNameKey(cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errResolveIdentity)
	}

	if err := c.client.UpdateInstance(ctx, key, cr); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateInstance)
	}

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

// Delete resolves cr's identity via keyForObserve, not the strict
// currentExternalNameKey Update uses: a deleting CR may still carry an
// empty or legacy annotation from adopting mid-deletion, which keyForObserve accepts.
// Ownership is not re-checked either: the reconciler only calls Delete after an
// Observe that returned ResourceExists, and Observe is where the guard runs.
func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*apisv1alpha1.Entitlement)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotEntitlement)
	}

	key, _, err := keyForObserve(cr)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errResolveIdentity)
	}

	instance, err := c.client.DescribeInstance(ctx, key)

	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDescribeInstance)
	}

	if c.updateInProgress(cr) {
		return managed.ExternalDelete{}, nil
	}

	c.tracker.SetConditions(ctx, cr)
	if blocked := c.tracker.DeleteShouldBeBlocked(mg); blocked {
		return managed.ExternalDelete{}, errors.New(providerv1alpha1.ErrResourceInUse)
	}

	entitlements, err := c.findRelatedEntitlements(
		ctx,
		cr,
		func(entitlement apisv1alpha1.Entitlement) bool { return entitlement.UID != cr.UID },
	)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errFindRelated)
	}
	cr.Status.AtProvider, err = entitlementclient.GenerateObservation(instance, entitlements)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errGenerateObservation)
	}

	if err := c.client.DeleteInstance(ctx, key, cr); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteInstance)
	}

	cr.SetConditions(xpv1.Deleting())
	return managed.ExternalDelete{}, nil
}

func (c *external) updateInProgress(cr *apisv1alpha1.Entitlement) bool {
	switch cr.Status.AtProvider.Assigned.EntityState { //nolint:exhaustive
	case apisv1alpha1.EntitlementStatusStarted:
		return true
	case apisv1alpha1.EntitlementStatusProcessing:
		return true
	}
	return false
}

func (c *external) needsUpdate(cr *apisv1alpha1.Entitlement) bool {
	// Just don't touch
	autoAssign := cr.Status.AtProvider.Assigned.AutoAssign
	if autoAssign {
		return false
	}
	// System-assigned entitlements are never resized or removed by us.
	autoAssigned := cr.Status.AtProvider.Assigned.AutoAssigned
	if autoAssigned {
		return false
	}
	unlimitedAmountAssigned := cr.Status.AtProvider.Assigned.UnlimitedAmountAssigned
	if unlimitedAmountAssigned {
		return false
	}

	if cr.Spec.ForProvider.Amount != nil {
		return !reflect.DeepEqual(cr.Status.AtProvider.Required.Amount, cr.Status.AtProvider.Assigned.Amount)
	}

	return false
}

// entitlementProcessingFailed reports whether BTP rejected or failed the last
// operation on this entitlement's service plan assignment.
func entitlementProcessingFailed(cr *apisv1alpha1.Entitlement) bool {
	return cr.Status.AtProvider != nil &&
		cr.Status.AtProvider.Assigned != nil &&
		cr.Status.AtProvider.Assigned.EntityState == apisv1alpha1.EntitlementStatusProcessingFailed
}

// entitlementFailedCondition builds the Ready=False condition for a
// PROCESSING_FAILED assignment, carrying BTP's own stateMessage so the
// rejection reason — for example a quota change below the currently consumed
// amount — is readable on the resource instead of being discarded.
func entitlementFailedCondition(cr *apisv1alpha1.Entitlement) xpv1.Condition {
	msg := "BTP reports the entitlement assignment as PROCESSING_FAILED"
	if cr.Status.AtProvider != nil && cr.Status.AtProvider.Assigned != nil {
		if sm := cr.Status.AtProvider.Assigned.StateMessage; sm != "" {
			msg += ": " + mrstatus.Truncate(sm, mrstatus.MaxMessageBytes)
		}
	}
	return mrstatus.ExternalResourceFailed(msg, cr.Generation)
}

// assignFailedNoQuota returns true when BTP reports a PROCESSING_FAILED
// assignment for this entitlement and the reported amount is zero or unset,
// i.e. nothing is actually reserved on the BTP side. This is distinct from
// PROCESSING_FAILED with a non-zero amount, which typically reflects a
// delete- or update-time failure on an entitlement that is still assigned:
// that one keeps its assignment and is only re-observed, not re-created.
//
// Neither is reported as Available any more. Since upstream issue #280 every
// PROCESSING_FAILED assignment carries Ready=False with BTP's own stateMessage
// (see entitlementFailedCondition); this predicate decides only whether the
// assignment has to be re-issued, not how it is reported.
func assignFailedNoQuota(cr *apisv1alpha1.Entitlement) bool {
	if cr.Status.AtProvider == nil || cr.Status.AtProvider.Assigned == nil {
		return false
	}
	if cr.Status.AtProvider.Assigned.EntityState != apisv1alpha1.EntitlementStatusProcessingFailed {
		return false
	}
	amount := cr.Status.AtProvider.Assigned.Amount
	return amount == nil || *amount <= 0
}

func (c *external) needsCreate(cr *apisv1alpha1.Entitlement) bool {
	if cr.Status.AtProvider.Assigned == nil {
		return true
	}
	// AutoAssigned entitlements are never (re)created: UpdateInstance
	// already refuses to write them, so treating PROCESSING_FAILED as
	// "not yet created" would loop Create forever. Self-adopt instead.
	if cr.Status.AtProvider.Assigned.AutoAssigned {
		return false
	}
	// Previous assign attempt failed and BTP reserved nothing — treat as
	// not-yet-created so Crossplane re-issues the assign via Create (which
	// calls CreateInstance == UpdateInstance) under the managed reconciler's
	// rate-limited retry.
	return assignFailedNoQuota(cr)
}

// deletionComplete checks whether this CR's portion has already been removed from BTP.
// When sibling CRs exist for the same service/plan, the BTP entitlement is reduced (not
// fully removed). We compare the current BTP assigned amount against the sum of the
// remaining sibling CRs to determine if our portion has been subtracted.
func (c *external) deletionComplete(ctx context.Context, cr *apisv1alpha1.Entitlement) (bool, error) {
	remainingEntitlements, err := c.findRelatedEntitlements(ctx, cr,
		func(e apisv1alpha1.Entitlement) bool { return e.UID != cr.UID },
	)
	if err != nil {
		return false, err
	}
	return deletionCompleteForSiblings(cr, remainingEntitlements)
}

// deletionCompleteForSiblings performs deletionComplete's comparison
// against an already-fetched sibling list; its zero-item shortcut always answers "not complete".
func deletionCompleteForSiblings(cr *apisv1alpha1.Entitlement, remainingEntitlements *apisv1alpha1.EntitlementList) (bool, error) {
	// No sibling CRs — Delete() must fully remove the assignment from BTP
	if len(remainingEntitlements.Items) == 0 {
		return false, nil
	}

	remainingRequired, err := entitlementclient.MergeRelatedEntitlements(remainingEntitlements)
	if err != nil {
		return false, err
	}

	// Numeric quota: BTP amount reduced to sibling sum means our portion is gone
	if remainingRequired.Amount != nil && cr.Status.AtProvider.Assigned.Amount != nil {
		return *cr.Status.AtProvider.Assigned.Amount <= *remainingRequired.Amount, nil
	}

	// Non-numeric (enable-based): siblings will continue managing it
	if remainingRequired.Enable != nil {
		return true, nil
	}

	return false, nil
}

// findRelatedEntitlements resolves all relevant entitlements which do not match the filter function and other static functions
func (c *external) findRelatedEntitlements(
	ctx context.Context,
	ours *apisv1alpha1.Entitlement,
	isRelevant func(entitlement apisv1alpha1.Entitlement) bool,
) (*apisv1alpha1.EntitlementList, error) {
	allEntitlements := &apisv1alpha1.EntitlementList{}
	err := c.kube.List(ctx, allEntitlements)

	if err != nil {
		return nil, errors.Wrap(err, errListEntitlements)
	}
	relatedEntitlements := &apisv1alpha1.EntitlementList{}
	for _, ent := range allEntitlements.Items {
		if !isRelevant(ent) {
			continue
		}
		if ent.Spec.ForProvider.SubaccountGuid != ours.Spec.ForProvider.SubaccountGuid {
			continue
		}
		if ent.Spec.ForProvider.ServiceName != ours.Spec.ForProvider.ServiceName {
			continue
		}
		if ent.Spec.ForProvider.ServicePlanName != ours.Spec.ForProvider.ServicePlanName {
			continue
		}
		// A qualifier identifies a distinct plan: nil matches only nil.
		entQualifier, ourQualifier := ent.Spec.ForProvider.ServicePlanUniqueIdentifier, ours.Spec.ForProvider.ServicePlanUniqueIdentifier
		if (entQualifier == nil) != (ourQualifier == nil) ||
			(entQualifier != nil && *entQualifier != *ourQualifier) {
			continue
		}
		if ent.GetCondition(xpv1.Deleting().Type).Reason == xpv1.Deleting().Reason {
			continue
		}
		relatedEntitlements.Items = append(relatedEntitlements.Items, ent)
	}
	return relatedEntitlements, nil
}

// softValidation adds conditions to the CR in order to guide the user with the usage of the Entitlements.
func (c *external) softValidation(cr *apisv1alpha1.Entitlement) xpv1.Condition {
	var errs []string
	if cr.Spec.ForProvider.Amount != nil && cr.Spec.ForProvider.Enable != nil {
		errs = append(errs, ".Spec.ForProvider.Amount & .Spec.ForProvider.Enable set. Only one value is supported. This depends on the type of service")
	}

	// Without further information, we cannot proceed, assuming issue with service calls
	if cr.Status.AtProvider == nil {
		return apisv1alpha1.ValidationCondition(errs)
	}
	if cr.Status.AtProvider.Entitled.Name == "" {
		errs = append(errs, "Could not find service to be entitled. Check if Global Account is entitled for usage (Control Center).")
	}

	if cr.Status.AtProvider.Entitled.Unlimited && cr.Status.AtProvider.Required.Amount != nil {
		errs = append(errs, "This serviceplan is non numeric, please use .Spec.ForProvider.Enable and omit the use of .Spec.ForProvider.Amount to configure the entitlement")
	}

	return apisv1alpha1.ValidationCondition(errs)
}
