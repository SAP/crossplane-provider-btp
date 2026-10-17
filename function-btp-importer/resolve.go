package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"

	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	xpmeta "github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/function-sdk-go/errors"
	"github.com/crossplane/function-sdk-go/logging"
	"github.com/crossplane/function-sdk-go/resource"

	"github.com/sap/crossplane-provider-btp/function-btp-importer/input/v1beta1"
	"github.com/sap/crossplane-provider-btp/function-btp-importer/internal/btp"
)

const (
	// lookupAnnotation is the opt-in annotation for mode=explicit.
	lookupAnnotation = "import.btp.sap.crossplane.io/lookup"

	// GVK group/version constants.
	accountV1Alpha1GroupVersion     = "account.btp.sap.crossplane.io/v1alpha1"
	accountV1Beta1GroupVersion      = "account.btp.sap.crossplane.io/v1beta1"
	environmentV1Alpha1GroupVersion = "environment.btp.sap.crossplane.io/v1alpha1"

	// Default instance/binding names for composite-key resources.
	defaultSMInstanceName = "managed-service-manager"
	defaultSMBindingName  = "managed-service-manager-binding"
	defaultCMInstanceName = "managed-cloud-management"
	defaultCMBindingName  = "managed-cloud-management-binding"

	// Kind constants.
	kindServiceInstance         = "ServiceInstance"
	kindServiceBinding          = "ServiceBinding"
	kindSubaccount              = "Subaccount"
	kindSubscription            = "Subscription"
	kindServiceManager          = "ServiceManager"
	kindCloudManagement         = "CloudManagement"
	kindKymaEnvironment         = "KymaEnvironment"
	kindCloudFoundryEnvironment = "CloudFoundryEnvironment"
)

// smClient defines the SM operations needed by the resolver.
type smClient interface {
	FindServiceInstance(ctx context.Context, name string) (*btp.SMResource, error)
	FindServiceBinding(ctx context.Context, name string) (*btp.SMResource, error)
	GetServicePlanIdentity(ctx context.Context, planID string) (offeringName, planName string, err error)
	Close() error
}

// accountsClient defines the Accounts Service operations needed by the resolver.
type accountsClient interface {
	FindSubaccount(ctx context.Context, subdomain, region string) (string, error)
}

// provisioningClient defines the Provisioning Service operations needed by the resolver.
type provisioningClient interface {
	FindKymaEnvironment(ctx context.Context, name, planName string) (string, error)
	FindCloudFoundryEnvironment(ctx context.Context, name string) (string, error)
}

// resolver holds the per-RunFunction-invocation state for resolving external names.
// It is created once in pass 2 and discarded after RunFunction returns.
// All methods are called from a single goroutine — no concurrent access.
type resolver struct {
	accounts     accountsClient                              // Accounts Service client (CIS OAuth)
	sm           smClient                                    // authenticated SM client; nil if no import candidates, no SM resources, or binding failed
	provisioning provisioningClient                          // Provisioning Service client (cloud management OAuth); nil if no env resources
	mode         v1beta1.Mode                                // auto or explicit
	include      []*regexp.Regexp                            // compiled include patterns; empty means include all
	exclude      []*regexp.Regexp                            // compiled exclude patterns
	observed     map[resource.Name]resource.ObservedComposed // observed composed resources; used to skip already-bound resources
	log          logging.Logger
	warn         func(error) // called for each non-fatal lookup error; maps to response.Warning in production

	// planCache collapses repeated plan-identity GETs within one invocation:
	// multiple instances often share a plan, and the import window re-renders
	// every reconcile until the observed external-name appears.
	planCache map[string]planIdentity
	// knownInstanceUUIDs holds the service instance UUIDs this composition
	// manages, collected before bindings resolve: instances stamped this pass
	// plus observed instance external-names. A name-matched binding is only
	// adopted when it belongs to one of these instances.
	knownInstanceUUIDs map[string]bool
}

// planIdentity is a cached offering/plan catalog-name pair for one plan ID.
type planIdentity struct {
	offering string
	plan     string
}

// identityMismatchError reports a name match that failed identity
// verification. It is warned and counted separately from lookup errors, and
// its message surfaces on the claim condition.
type identityMismatchError struct{ msg string }

func (e *identityMismatchError) Error() string { return e.msg }

// errBindingDeferred marks a name-matched binding that cannot be verified yet
// because no composition-managed instance UUID is known this pass. It is
// skipped quietly; resolution converges once an instance resolves.
var errBindingDeferred = errors.New("binding import deferred until a composition-managed instance is known")

// resolveConfig holds the inputs needed to construct a resolver.
type resolveConfig struct {
	Creds    *btp.CISCredentials
	Observed map[resource.Name]resource.ObservedComposed
	Desired  map[resource.Name]*resource.DesiredComposed
	Input    *v1beta1.Input
	Log      logging.Logger
	Warn     func(error)
}

// newResolver constructs a fully-assembled resolver from raw credentials
// and pipeline state. It builds all API clients needed for resolution:
//   - CIS client (always)
//   - SM client (if observed resources have a resolved subaccountId)
//   - Provisioning client (if SM available and environment resources in desired)
//
// Returns a non-nil error only for fatal failures (e.g. auth errors).
// Transient failures are reported via cfg.Warn and the affected client stays nil.
func newResolver(ctx context.Context, cfg resolveConfig) (*resolver, error) {
	includePatterns, err := compilePatterns(cfg.Input.Include)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid include pattern")
	}
	excludePatterns, err := compilePatterns(cfg.Input.Exclude)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid exclude pattern")
	}

	cisClient := btp.NewOAuthClient(ctx, cfg.Creds.UAA.URL+"/oauth/token", cfg.Creds.UAA.ClientID, cfg.Creds.UAA.ClientSecret)
	accounts := btp.NewAccountsClient(cisClient, cfg.Creds.Endpoints.AccountsServiceURL)

	// Acquire the SM admin binding only when something actually needs a lookup.
	// The CIS v1 SM binding is a per-subaccount singleton shared with provider-btp:
	// acquiring (and creating/releasing) it on reconciles with zero import candidates
	// races the other holder (409) and invalidates its credentials (2026-08-04).
	// On steady-state landscapes every resource is already bound, so SM is skipped
	// entirely.
	var sm smClient
	var provisioning provisioningClient
	var smc *btp.SMClient
	if anyLookupCandidates(cfg.Desired, cfg.Observed, cfg.Input.Mode, includePatterns, excludePatterns, cfg.Log) {
		// The "no subaccount known yet -> no SM client" policy lives here, next
		// to where the subaccount ID is computed: the constructor either
		// succeeds or fails, it never silently returns nothing.
		if subaccountID := smSubaccountID(cfg.Observed); subaccountID != "" {
			var smErr error
			smc, smErr = btp.NewSMClient(ctx, cisClient, cfg.Creds.Endpoints.AccountsServiceURL, subaccountID)
			if smErr != nil {
				var authErr *btp.SMAuthError
				if errors.As(smErr, &authErr) {
					return nil, smErr
				}
				// Intentionally includes consequence ("skipped this reconcile") — this
				// surfaces as a Crossplane Warning condition read by operators, so the
				// impact context is more useful than cause-only.
				cfg.Warn(errors.Wrap(smErr, "cannot acquire SM admin binding; SM resources skipped this reconcile"))
			}
		}
	} else {
		cfg.Log.Debug("No import candidates; skipping SM binding acquisition")
	}

	// Avoid nil-interface gotcha: a nil *btp.SMClient assigned to smClient
	// would be non-nil. Only assign when the concrete value is non-nil.
	if smc != nil {
		sm = smc
	}

	if smc != nil {
		if cmBindingName := envBindingName(cfg.Desired, cfg.Observed); cmBindingName != "" {
			cmCreds, cmErr := smc.GetBindingCredentials(ctx, cmBindingName)
			switch {
			case cmErr != nil:
				// Intentionally includes consequence — operator-facing Warning, see above.
				cfg.Warn(errors.Wrapf(cmErr, "cannot get cloud management credentials; environment resources skipped this reconcile"))
			case cmCreds.Endpoints.ProvisioningServiceURL == "":
				cfg.Warn(errors.New("cloud management credentials missing provisioning_service_url; environment resources skipped this reconcile"))
			default:
				provClient := btp.NewOAuthClient(ctx, cmCreds.UAA.URL+"/oauth/token", cmCreds.UAA.ClientID, cmCreds.UAA.ClientSecret)
				provisioning = btp.NewProvisioningClient(provClient, cmCreds.Endpoints.ProvisioningServiceURL)
			}
		}
	}

	return &resolver{
		accounts:     accounts,
		sm:           sm,
		provisioning: provisioning,
		mode:         cfg.Input.Mode,
		include:      includePatterns,
		exclude:      excludePatterns,
		observed:     cfg.Observed,
		log:          cfg.Log,
		warn:         cfg.Warn,
	}, nil
}

func (r *resolver) Close() error {
	if r.sm != nil {
		return r.sm.Close()
	}
	return nil
}

// anyLookupCandidates reports whether any supported desired resource still
// needs a BTP lookup. Used to avoid acquiring the shared SM admin binding on
// reconciles where nothing can be imported.
func anyLookupCandidates(desired map[resource.Name]*resource.DesiredComposed, observed map[resource.Name]resource.ObservedComposed, mode v1beta1.Mode, include, exclude []*regexp.Regexp, log logging.Logger) bool {
	for resName, dcd := range desired {
		if !isSupportedGVK(dcd.Resource.GetAPIVersion(), dcd.Resource.GetKind()) {
			continue
		}
		if needsLookup(resName, dcd, observed, mode, include, exclude, log) {
			return true
		}
	}
	return false
}

// needsLookup reports whether a supported desired resource still needs a BTP
// lookup. The OBSERVED composed resource is consulted first: an observed MR
// with a real external-name is already bound, and re-looking it up every
// reconcile produced permanent lookup noise against live landscapes
// (2026-08-04) — the desired render never carries the provider-set
// external-name, so checking desired alone never skips managed resources.
func needsLookup(resName resource.Name, dcd *resource.DesiredComposed, observed map[resource.Name]resource.ObservedComposed, mode v1beta1.Mode, include, exclude []*regexp.Regexp, log logging.Logger) bool {
	name := string(resName)
	kind := dcd.Resource.GetKind()

	if en := observedExternalName(observed, resName); en != "" {
		log.Debug("Skipping resource: observed external-name already set", "resourceName", name, "kind", kind, "externalName", en)
		return false
	}

	// provider-btp does not backfill a default external-name: empty means
	// "create", set means "already adopted". The one exception is the legacy
	// KymaEnvironment format, where the external-name equals metadata.name
	// (the provider migrates it to the GUID); treat that as not yet imported
	// and proceed with the lookup.
	if en := xpmeta.GetExternalName(dcd.Resource); en != "" && en != dcd.Resource.GetName() {
		log.Debug("Skipping resource: external-name already set", "resourceName", name, "kind", kind, "externalName", en)
		return false
	}

	if ok, reason := importCandidate(name, dcd, mode, include, exclude); !ok {
		log.Debug("Skipping resource: "+reason, "resourceName", name, "kind", kind)
		return false
	}

	return true
}

// observedExternalName returns the observed composed resource's external-name
// when it is a real, resolved identity: non-empty and different from
// metadata.name (the legacy KymaEnvironment format, not a resolved GUID).
func observedExternalName(observed map[resource.Name]resource.ObservedComposed, resName resource.Name) string {
	ocd, ok := observed[resName]
	if !ok {
		return ""
	}
	if en := xpmeta.GetExternalName(ocd.Resource); en != "" && en != ocd.Resource.GetName() {
		return en
	}
	return ""
}

// importCandidate reports whether a resource is in the population this
// function manages: opted in (mode=explicit requires the lookup annotation)
// and not ruled out by the include/exclude filters. Pure predicate; the
// second return is the skip reason for the caller to log. Both the lookup
// decision and observed external-name propagation are scoped by it so the
// two gates cannot drift apart.
func importCandidate(name string, dcd *resource.DesiredComposed, mode v1beta1.Mode, include, exclude []*regexp.Regexp) (bool, string) {
	if mode == v1beta1.ModeExplicit {
		if dcd.Resource.GetAnnotations()[lookupAnnotation] != "true" {
			return false, "mode=explicit and lookup annotation absent"
		}
	}

	if len(include) > 0 && !matchesAny(name, include) {
		return false, "not matched by include filter"
	}

	if matchesAny(name, exclude) {
		return false, "matched by exclude filter"
	}

	return true, ""
}

// resolveSummary holds per-run counters for the pass-2 completion log.
type resolveSummary struct {
	total      int // total supported resources considered
	imported   int // external-name set by this run
	skipped    int // skipped (already set, explicit mode, filters)
	propagated int // observed external-name copied into the desired render (adopted resources)
	noMatches  int // queried but no BTP result found
	warnings   int // lookup errors that became Warnings

	identityMismatch int      // name matched but identity verification refused the adoption
	blocked          []string // one message per refused adoption, surfaced on the claim condition
}

// resolveAll iterates over all desired composed resources and sets
// crossplane.io/external-name on those that can be matched in the BTP API.
// It runs in two phases: every kind except ServiceBinding resolves first,
// then bindings — so binding verification always sees the complete set of
// instance UUIDs this composition manages, regardless of map iteration
// order. Within each phase the order is sorted for stable logs.
func (r *resolver) resolveAll(ctx context.Context, desired map[resource.Name]*resource.DesiredComposed) resolveSummary {
	var s resolveSummary

	r.knownInstanceUUIDs = observedInstanceUUIDs(r.observed)

	names := make([]resource.Name, 0, len(desired))
	for resName := range desired {
		names = append(names, resName)
	}
	slices.Sort(names)

	for _, resName := range names {
		if desired[resName].Resource.GetKind() != kindServiceBinding {
			r.resolveOne(ctx, &s, resName, desired[resName])
		}
	}
	for _, resName := range names {
		if desired[resName].Resource.GetKind() == kindServiceBinding {
			r.resolveOne(ctx, &s, resName, desired[resName])
		}
	}

	return s
}

// observedInstanceUUIDs collects the real external-names of observed
// ServiceInstances — the instances this composition already manages.
func observedInstanceUUIDs(observed map[resource.Name]resource.ObservedComposed) map[string]bool {
	uuids := map[string]bool{}
	for resName, ocd := range observed {
		if ocd.Resource.GetKind() != kindServiceInstance || ocd.Resource.GetAPIVersion() != accountV1Alpha1GroupVersion {
			continue
		}
		if en := observedExternalName(observed, resName); en != "" {
			uuids[en] = true
		}
	}
	return uuids
}

// resolveOne resolves the external-name of a single desired composed
// resource, updating the summary counters.
func (r *resolver) resolveOne(ctx context.Context, s *resolveSummary, resName resource.Name, dcd *resource.DesiredComposed) {
	name := string(resName)

	apiVersion := dcd.Resource.GetAPIVersion()
	kind := dcd.Resource.GetKind()

	if !isSupportedGVK(apiVersion, kind) {
		r.log.Debug("Skipping resource with unknown GVK", "resourceName", name, "apiVersion", apiVersion, "kind", kind)
		return
	}

	s.total++

	// Adopted resources: this function stamped the external-name into the
	// render on an earlier reconcile, so the COMPOSITION's field manager
	// owns the annotation on the MR. Desired state is re-rendered from
	// scratch every reconcile — if the observed-aware skip below left the
	// annotation out, server-side apply would remove it, crossplane would
	// backfill it with metadata.name, and the next reconcile would
	// re-look-up and re-stamp: a permanent skip/import oscillation
	// (live-hit 2026-08-13: ~300ms XR hot loop, Synced flapping, and
	// duplicate-Create risk under FullControl). Skipping the LOOKUP must
	// therefore still carry the known VALUE forward. Scoped by
	// importCandidate — the exact population the lookup targets — so
	// filtered-out and (in explicit mode) unannotated resources stay
	// untouched, and a composition-set external-name in the render is
	// never overridden. Rotation-managed bindings must be OUTSIDE this
	// population in every mode (unannotated or excluded): lookups cannot
	// match their suffixed names, and mirroring a rotating external-name
	// can revert the provider's rotation write. Compositions that
	// blanket-annotate (existingResourcePolicy: Adopt) must not enable
	// rotation on annotated bindings.
	if en := observedExternalName(r.observed, resName); en != "" {
		if cur := xpmeta.GetExternalName(dcd.Resource); cur == "" || cur == dcd.Resource.GetName() {
			if ok, _ := importCandidate(name, dcd, r.mode, r.include, r.exclude); ok {
				xpmeta.SetExternalName(dcd.Resource, en)
				r.log.Debug("Propagated observed external-name into desired", "resourceName", name, "kind", kind, "externalName", en)
				s.propagated++
			}
		}
	}

	if !needsLookup(resName, dcd, r.observed, r.mode, r.include, r.exclude, r.log) {
		s.skipped++
		return
	}

	uuid, err := r.lookupResource(ctx, name, apiVersion, kind, dcd)
	if err != nil {
		var mismatch *identityMismatchError
		switch {
		case errors.As(err, &mismatch):
			r.warn(err)
			s.identityMismatch++
			s.blocked = append(s.blocked, err.Error())
		case errors.Is(err, errBindingDeferred):
			// Already debug-logged at the decision site. Counted as skipped:
			// nothing was imported and nothing is wrong — the next pass with
			// a resolved instance converges.
			s.skipped++
		default:
			r.warn(errors.Wrapf(err, "lookup failed for resource %q (%s)", name, kind))
			s.warnings++
		}
		return
	}
	if uuid == "" {
		r.log.Debug("No match found in BTP, provider will create", "resourceName", name, "kind", kind)
		s.noMatches++
		return
	}

	xpmeta.SetExternalName(dcd.Resource, uuid)
	if kind == kindServiceInstance {
		r.knownInstanceUUIDs[uuid] = true
	}
	r.log.Info("Imported existing BTP resource", "resourceName", name, "kind", kind, "externalName", uuid)
	s.imported++
}

// gvk identifies a composed resource type by apiVersion and kind.
type gvk struct{ apiVersion, kind string }

// lookupFunc resolves the external-name for one supported GVK. All entries
// share one signature so the dispatch table can hold them as method
// expressions, which is why the receiver precedes ctx. The kind parameter
// lets lookups that serve several kinds (composite-key resources,
// environments) discriminate; single-kind lookups ignore it.
type lookupFunc func(r *resolver, ctx context.Context, resName, kind string, paved *fieldpath.Paved) (string, error)

// lookups is the single source of truth for supported GVKs: a resource type
// is supported exactly when it has an entry here, and that entry is how it
// is dispatched, so "supported" and "dispatched" cannot drift apart.
//
//nolint:gochecknoglobals // single source of truth for supported GVKs; read-only after init
var lookups = map[gvk]lookupFunc{
	{apiVersion: accountV1Alpha1GroupVersion, kind: kindServiceInstance}:             (*resolver).lookupServiceInstance,
	{apiVersion: accountV1Alpha1GroupVersion, kind: kindServiceBinding}:              (*resolver).lookupServiceBinding,
	{apiVersion: accountV1Alpha1GroupVersion, kind: kindSubaccount}:                  (*resolver).lookupSubaccount,
	{apiVersion: accountV1Alpha1GroupVersion, kind: kindSubscription}:                (*resolver).lookupSubscription,
	{apiVersion: accountV1Alpha1GroupVersion, kind: kindServiceManager}:              (*resolver).lookupCompositeKeyResource,
	{apiVersion: accountV1Alpha1GroupVersion, kind: kindCloudManagement}:             (*resolver).lookupCompositeKeyResource,
	{apiVersion: accountV1Beta1GroupVersion, kind: kindServiceManager}:               (*resolver).lookupCompositeKeyResource,
	{apiVersion: accountV1Beta1GroupVersion, kind: kindCloudManagement}:              (*resolver).lookupCompositeKeyResource,
	{apiVersion: environmentV1Alpha1GroupVersion, kind: kindKymaEnvironment}:         (*resolver).lookupEnvironment,
	{apiVersion: environmentV1Alpha1GroupVersion, kind: kindCloudFoundryEnvironment}: (*resolver).lookupEnvironment,
}

func isSupportedGVK(apiVersion, kind string) bool {
	_, ok := lookups[gvk{apiVersion: apiVersion, kind: kind}]
	return ok
}

// lookupResource dispatches to the appropriate BTP API based on apiVersion/kind.
// Returns the UUID (or "") if no match, or an error for API failures. Callers
// filter through isSupportedGVK first, and both consult the same table, so
// the entry always exists.
func (r *resolver) lookupResource(ctx context.Context, resName, apiVersion, kind string, dcd *resource.DesiredComposed) (string, error) {
	return lookups[gvk{apiVersion: apiVersion, kind: kind}](r, ctx, resName, kind, fieldpath.Pave(dcd.Resource.Object))
}

// smResourceName guards that the SM client is available and reads the
// spec.forProvider.name that an SM name lookup keys on.
func (r *resolver) smResourceName(resName string, paved *fieldpath.Paved) (string, error) {
	if err := r.requireSMClient(); err != nil {
		return "", err
	}
	name, err := paved.GetString("spec.forProvider.name")
	if err != nil {
		return "", errors.Wrapf(err, "cannot get spec.forProvider.name for resource %q", resName)
	}
	return name, nil
}

func (r *resolver) lookupServiceInstance(ctx context.Context, resName, _ string, paved *fieldpath.Paved) (string, error) {
	name, err := r.smResourceName(resName, paved)
	if err != nil {
		return "", err
	}
	found, err := r.sm.FindServiceInstance(ctx, name)
	if err != nil || found == nil {
		return "", err
	}
	return r.verifyInstanceIdentity(ctx, name, found, paved)
}

func (r *resolver) lookupServiceBinding(ctx context.Context, resName, _ string, paved *fieldpath.Paved) (string, error) {
	name, err := r.smResourceName(resName, paved)
	if err != nil {
		return "", err
	}
	found, err := r.sm.FindServiceBinding(ctx, name)
	if err != nil || found == nil {
		return "", err
	}
	if r.knownInstanceUUIDs[found.ServiceInstanceID] {
		return found.ID, nil
	}
	if len(r.knownInstanceUUIDs) == 0 {
		r.log.Debug("Binding name matched but no composition-managed instance is known yet; deferring import to a later pass", "bindingName", name)
		return "", errBindingDeferred
	}
	return "", &identityMismatchError{msg: fmt.Sprintf("binding %q belongs to instance %s, not managed by this composition; skipping import", name, found.ServiceInstanceID)}
}

func (r *resolver) requireSMClient() error {
	if r.sm == nil {
		return errors.New("SM client not available")
	}
	return nil
}

// verifyInstanceIdentity accepts a name-matched instance only when its plan
// resolves to the offering/plan names the spec declares. A spec that declares
// neither cannot be verified and is adopted as a plain name match — kept for
// compositions that do not render the identity fields. Any declared identity
// is verified in full: a partial declaration that disagrees with the resolved
// identity blocks the import rather than half-passing.
func (r *resolver) verifyInstanceIdentity(ctx context.Context, name string, found *btp.SMResource, paved *fieldpath.Paved) (string, error) {
	wantOffering, err := paved.GetString("spec.forProvider.offeringName")
	if err != nil && !fieldpath.IsNotFound(err) {
		return "", errors.Wrap(err, "cannot get spec.forProvider.offeringName")
	}
	wantPlan, err := paved.GetString("spec.forProvider.planName")
	if err != nil && !fieldpath.IsNotFound(err) {
		return "", errors.Wrap(err, "cannot get spec.forProvider.planName")
	}
	if wantOffering == "" && wantPlan == "" {
		r.log.Debug("Instance spec declares no offering/plan names; adopting name match without identity verification", "instanceName", name)
		return found.ID, nil
	}

	gotOffering, gotPlan, err := r.planIdentity(ctx, found.ServicePlanID)
	if err != nil {
		return "", &identityMismatchError{msg: fmt.Sprintf("instance %q exists but its plan identity could not be verified: %s; skipping import", name, err)}
	}
	if gotOffering != wantOffering || gotPlan != wantPlan {
		return "", &identityMismatchError{msg: fmt.Sprintf("instance %q exists but is %s/%s, expected %s/%s; skipping import", name, gotOffering, gotPlan, wantOffering, wantPlan)}
	}
	return found.ID, nil
}

// planIdentity resolves a plan ID to its catalog names through the per-pass
// cache.
func (r *resolver) planIdentity(ctx context.Context, planID string) (offering, plan string, err error) {
	if identity, ok := r.planCache[planID]; ok {
		return identity.offering, identity.plan, nil
	}
	offering, plan, err = r.sm.GetServicePlanIdentity(ctx, planID)
	if err != nil {
		return "", "", err
	}
	if r.planCache == nil {
		r.planCache = map[string]planIdentity{}
	}
	r.planCache[planID] = planIdentity{offering: offering, plan: plan}
	return offering, plan, nil
}

func (r *resolver) lookupSubaccount(ctx context.Context, resName, _ string, paved *fieldpath.Paved) (string, error) {
	subdomain, err := paved.GetString("spec.forProvider.subdomain")
	if err != nil {
		return "", errors.Wrapf(err, "cannot get spec.forProvider.subdomain for resource %q", resName)
	}
	region, err := paved.GetString("spec.forProvider.region")
	if err != nil {
		return "", errors.Wrapf(err, "cannot get spec.forProvider.region for resource %q", resName)
	}
	return r.accounts.FindSubaccount(ctx, subdomain, region)
}

// lookupSubscription derives the external-name from the desired spec alone.
// The provider identifies a Subscription by "<appName>/<planName>" — the same
// value it writes as the external-name after its own Create — so the stamp is
// deterministic and needs no BTP call, no client and no observed state. The
// stamp is an identity key, not a confirmed existence: whether the
// subscription exists is left to the provider's Observe (present → adopted,
// absent → created under this same name). It therefore counts as imported
// like every other kind, even though nothing was looked up.
//
// planName may legitimately be the empty string (the cockpit "default" plan),
// producing "appName/" with a trailing slash, exactly as the provider forms
// it. A MISSING planName key is different: that is CRD-invalid input and is
// reported rather than silently turned into "appName/".
//
// Known trade-off: a Subscription that still needs stamping counts as a
// lookup candidate in anyLookupCandidates, so on a composition where every
// other resource is already adopted, adding a Subscription makes the resolver
// acquire the SM admin binding once even though this lookup never uses it.
// It happens once (the next reconcile skips on the observed external-name),
// never per reconcile. Excluding spec-derived kinds from the candidate check
// would avoid that single acquisition at the cost of a second "what does this
// kind need" rule beside the lookup table; change it here if it ever matters.
func (r *resolver) lookupSubscription(_ context.Context, resName, _ string, paved *fieldpath.Paved) (string, error) {
	appName, err := paved.GetString("spec.forProvider.appName")
	if err != nil {
		return "", errors.Wrapf(err, "cannot get spec.forProvider.appName for resource %q", resName)
	}
	if appName == "" {
		return "", errors.Errorf("spec.forProvider.appName is empty for resource %q", resName)
	}
	planName, err := paved.GetString("spec.forProvider.planName")
	if err != nil {
		return "", errors.Wrapf(err, "cannot get spec.forProvider.planName for resource %q", resName)
	}
	return appName + "/" + planName, nil
}

func (r *resolver) lookupCompositeKeyResource(ctx context.Context, resName, kind string, paved *fieldpath.Paved) (string, error) {
	if err := r.requireSMClient(); err != nil {
		return "", err
	}
	defaultInst, defaultBind := defaultSMInstanceName, defaultSMBindingName
	if kind == kindCloudManagement {
		defaultInst, defaultBind = defaultCMInstanceName, defaultCMBindingName
	}
	instName, err := paved.GetString("spec.forProvider.serviceInstanceName")
	if err != nil && !fieldpath.IsNotFound(err) {
		return "", errors.Wrapf(err, "cannot get spec.forProvider.serviceInstanceName for resource %q", resName)
	}
	if instName == "" {
		instName = defaultInst
	}
	bindName, err := paved.GetString("spec.forProvider.serviceBindingName")
	if err != nil && !fieldpath.IsNotFound(err) {
		return "", errors.Wrapf(err, "cannot get spec.forProvider.serviceBindingName for resource %q", resName)
	}
	if bindName == "" {
		bindName = defaultBind
	}
	return r.lookupCompositeKey(ctx, resName, kind, instName, bindName)
}

// lookupEnvironment uses the cloud management provisioning client, not the CIS client.
//
// The field path asymmetry between Kyma and CF is intentional — it mirrors the
// provider-btp CRD schema: KymaEnvironment uses spec.forProvider.name and
// spec.forProvider.planName; CloudFoundryEnvironment uses
// spec.forProvider.environmentName and has no plan field.
// Ref: https://github.com/SAP/crossplane-provider-btp/blob/main/apis/environment/v1alpha1/kymaenvironment_types.go
func (r *resolver) lookupEnvironment(ctx context.Context, resName, kind string, paved *fieldpath.Paved) (string, error) {
	if r.provisioning == nil {
		return "", errors.New("provisioning client not available")
	}

	switch kind {
	case kindCloudFoundryEnvironment:
		name, err := paved.GetString("spec.forProvider.environmentName")
		if err != nil {
			return "", errors.Wrapf(err, "cannot get spec.forProvider.environmentName for resource %q", resName)
		}
		return r.provisioning.FindCloudFoundryEnvironment(ctx, name)

	case kindKymaEnvironment:
		name, err := paved.GetString("spec.forProvider.name")
		if err != nil {
			return "", errors.Wrapf(err, "cannot get spec.forProvider.name for resource %q", resName)
		}
		// spec.forProvider.name is optional in the CRD — if absent, the provider
		// falls back to metadata.name (the composed resource name). We do not
		// implement that fallback here: for import to work the BTP environment name
		// must be known, and metadata.name on a composed resource is a generated
		// value that would not match an existing BTP environment. Operators must
		// set spec.forProvider.name explicitly when using the importer.
		planName, err := paved.GetString("spec.forProvider.planName")
		if err != nil {
			return "", errors.Wrapf(err, "cannot get spec.forProvider.planName for resource %q", resName)
		}
		return r.provisioning.FindKymaEnvironment(ctx, name, planName)

	default:
		return "", errors.Errorf("unsupported environment kind %q", kind)
	}
}

// lookupCompositeKey returns "instanceID/bindingID", or "" if either is not found.
func (r *resolver) lookupCompositeKey(ctx context.Context, resName, kind, instName, bindName string) (string, error) {
	inst, err := r.sm.FindServiceInstance(ctx, instName)
	if err != nil {
		return "", errors.Wrapf(err, "instance lookup failed for %s %q", kind, resName)
	}
	if inst == nil {
		r.log.Debug("No instance match found, provider will create", "resourceName", resName, "kind", kind, "instanceName", instName)
		return "", nil
	}

	bind, err := r.sm.FindServiceBinding(ctx, bindName)
	if err != nil {
		return "", errors.Wrapf(err, "binding lookup failed for %s %q", kind, resName)
	}
	if bind == nil {
		r.log.Debug("Instance found but no binding match, provider will create", "resourceName", resName, "kind", kind, "bindingName", bindName)
		return "", nil
	}

	if bind.ServiceInstanceID != inst.ID {
		return "", &identityMismatchError{msg: fmt.Sprintf("binding %q belongs to instance %s, not %s; skipping import", bindName, bind.ServiceInstanceID, inst.ID)}
	}

	return inst.ID + "/" + bind.ID, nil
}

func matchesAny(name string, patterns []*regexp.Regexp) bool {
	for _, p := range patterns {
		if p.MatchString(name) {
			return true
		}
	}
	return false
}

// smSubaccountID returns the subaccount ID from the first SM-related resource
// found in the observed map, or "" if none exist yet. The provider resolves
// subaccountSelector into subaccountId (v1alpha1) or subaccountGuid (v1beta1),
// so it is only present on observed resources — never in the desired map from
// the composition template.
//
// Field-path errors are intentionally ignored — a missing or malformed
// subaccountId/subaccountGuid is treated as "not yet observed" rather than
// a fatal misconfiguration, consistent with best-effort discovery semantics.
func smSubaccountID(observed map[resource.Name]resource.ObservedComposed) string {
	for _, ocd := range observed {
		apiVersion := ocd.Resource.GetAPIVersion()
		kind := ocd.Resource.GetKind()

		if !isSupportedGVK(apiVersion, kind) {
			continue
		}

		paved := fieldpath.Pave(ocd.Resource.Object)

		var id string
		var err error

		switch kind {
		case kindServiceInstance, kindServiceBinding:
			id, err = paved.GetString("spec.forProvider.subaccountId")
		case kindServiceManager, kindCloudManagement:
			id, err = paved.GetString("spec.forProvider.subaccountGuid")
		case kindKymaEnvironment, kindCloudFoundryEnvironment:
			// Environment resources expose subaccountGuid as a top-level spec
			// field — set by the provider from subaccountSelector/subaccountRef,
			// not by the composition template.
			id, err = paved.GetString("spec.subaccountGuid")
		default:
			continue
		}

		if err == nil && id != "" {
			return id
		}
	}
	return ""
}

// envBindingName returns the cloud management binding name to use for
// acquiring Provisioning API credentials, or "" if no environment resources
// (KymaEnvironment, CloudFoundryEnvironment) are present in the desired map.
func envBindingName(desired map[resource.Name]*resource.DesiredComposed, observed map[resource.Name]resource.ObservedComposed) string {
	hasEnv := false
	bindingName := ""

	for _, dcd := range desired {
		apiVersion := dcd.Resource.GetAPIVersion()
		kind := dcd.Resource.GetKind()

		if (kind == kindKymaEnvironment || kind == kindCloudFoundryEnvironment) && apiVersion == environmentV1Alpha1GroupVersion {
			hasEnv = true
		}
		if kind == kindCloudManagement && (apiVersion == accountV1Alpha1GroupVersion || apiVersion == accountV1Beta1GroupVersion) && bindingName == "" {
			name, err := fieldpath.Pave(dcd.Resource.Object).GetString("spec.forProvider.serviceBindingName")
			if err == nil && name != "" {
				bindingName = name
			}
		}
	}

	if !hasEnv {
		return ""
	}

	if bindingName != "" {
		return bindingName
	}

	for _, ocd := range observed {
		if ocd.Resource.GetKind() == kindCloudManagement {
			apiVersion := ocd.Resource.GetAPIVersion()
			if apiVersion == accountV1Alpha1GroupVersion || apiVersion == accountV1Beta1GroupVersion {
				name, err := fieldpath.Pave(ocd.Resource.Object).GetString("spec.forProvider.serviceBindingName")
				if err == nil && name != "" {
					return name
				}
			}
		}
	}

	return defaultCMBindingName
}

// compilePatterns compiles a slice of RE2 regex strings into *regexp.Regexp.
func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid regex pattern %q", p)
		}
		compiled = append(compiled, re)
	}
	return compiled, nil
}
