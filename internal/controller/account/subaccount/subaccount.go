package subaccount

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
	"github.com/sap/crossplane-provider-btp/internal/recovery"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotSubaccount        = "managed resource is not a Subaccount custom resource"
	subaccountStateDeleting = "DELETING"
	subaccountStateOk       = "OK"
	errConnect              = "while connecting to provider"
	errObserve              = "while observing subaccount"
	errMigrateExternalName  = "while migrating external name"
	errInvalidExternalName  = "external-name is not a valid GUID format"
	errGenerateObservation  = "while generating observation"
	errCreate               = "while creating subaccount"
	errUpdate               = "while updating subaccount"
	errUpdateAPI            = "while updating subaccount via API"
	errMoveSubaccount       = "while moving subaccount"
	errDelete               = "while deleting subaccount"
	errGetSubaccounts       = "while getting subaccounts"
	errUpdateExternalName   = "while updating external name"
)

// btpErrCodeActiveServiceInstances is BTP's error code for "You can't delete
// subaccounts with active service instances". The API does not name the
// instances, which leaves an operator with no way to find out what blocks the
// teardown - especially when those instances have no managed resource of their
// own.
const btpErrCodeActiveServiceInstances = 70011

// maxListedBlockers bounds how many blocking service instances are named in
// the surfaced error, so a subaccount with hundreds of instances cannot
// produce an unbounded condition message.
const maxListedBlockers = 10

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	usage           providerconfig.LegacyTracker
	resourcetracker tracking.ReferenceResolverTracker

	newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)

	// newInstanceListerFn builds a read-only lister for the service instances
	// in a subaccount from an already-connected BTP client, used to name what
	// blocks a refused deletion. Overridable for tests; when nil, Connect
	// installs the default accounts-service-backed implementation.
	newInstanceListerFn func(btpClient *btp.Client) func(ctx context.Context, subaccountGuid string) (smClient.InstanceLister, error)

	// recorder emits Kubernetes events for the heal path. May be nil.
	recorder event.Recorder
}

// defaultInstanceListerFn builds the read-only service-instance lister from
// the subaccount's EXISTING service-manager admin binding.
//
// It deliberately does not go through EnsureSemanticLookuper, which MINTS a
// temporary admin binding when none exists: on the delete path that would add
// another service instance to the very subaccount whose deletion is blocked by
// service instances.
func defaultInstanceListerFn(btpClient *btp.Client) func(ctx context.Context, subaccountGuid string) (smClient.InstanceLister, error) {
	return func(ctx context.Context, subaccountGuid string) (smClient.InstanceLister, error) {
		return smClient.NewServiceManagerInstanceProxyClient(btpClient.AccountsServiceClient).
			InstanceLister(ctx, subaccountGuid)
	}
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	_, ok := mg.(*apisv1alpha1.Subaccount)
	if !ok {
		return nil, errors.New(errNotSubaccount)
	}

	btpclient, err := providerconfig.CreateClient(ctx, mg, c.kube, c.usage, c.newServiceFn, c.resourcetracker)
	if err != nil {
		return nil, errors.Wrap(err, errConnect)
	}

	newInstanceListerFn := c.newInstanceListerFn
	if newInstanceListerFn == nil {
		newInstanceListerFn = defaultInstanceListerFn
	}

	return &external{
		Client:              c.kube,
		btp:                 *btpclient,
		tracker:             c.resourcetracker,
		accountsAccessor:    &AccountsClient{btp: *btpclient},
		newInstanceListerFn: newInstanceListerFn(btpclient),
		recorder:            c.recorder,
	}, nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	// A 'client' used to connect to the external resource API. In practice this
	// would be something like an AWS SDK client.
	client.Client
	btp     btp.Client
	tracker tracking.ReferenceResolverTracker

	accountsAccessor AccountsApiAccessor

	// newInstanceListerFn builds a read-only lister for the service instances
	// in a subaccount, used to name what blocks a refused deletion. May be nil.
	newInstanceListerFn func(ctx context.Context, subaccountGuid string) (smClient.InstanceLister, error)

	// recorder emits Kubernetes events for the heal path. May be nil.
	recorder event.Recorder
}

// Disconnect is a no-op for the external client to close its connection.
// Since we dont need this, we only have it to fullfil the interface.
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	desiredCR, ok := mg.(*apisv1alpha1.Subaccount)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotSubaccount)
	}

	// ADR Step 1: Check if external-name is empty
	if meta.GetExternalName(desiredCR) == "" {
		// Recovery also covers the delete leg: healing here lets the next
		// reconcile's Delete target the real subaccount instead of stripping
		// the finalizer and orphaning it.
		if hErr := c.healExternalName(ctx, desiredCR); hErr != nil {
			return managed.ExternalObservation{}, hErr
		}
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil

	}

	// Migrate old external-name format if necessary
	if err := c.migrateExternalName(ctx, desiredCR); err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errMigrateExternalName)
	}

	// ADR Step 2: External-name is set, check its format (must be valid GUID)
	if !isValidUUID(meta.GetExternalName(desiredCR)) {
		return managed.ExternalObservation{}, errors.Wrap(errors.New(fmt.Sprintf("external-name '%s'", meta.GetExternalName(desiredCR))), errInvalidExternalName)
	}

	// ADR Step 3: Build the Get API Request from the external-name (using GUID directly)
	if err := c.generateObservation(ctx, desiredCR); err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGenerateObservation)
	}

	c.tracker.SetConditions(ctx, desiredCR)

	// Needs Creation?
	if needsCreation := c.needsCreation(desiredCR); needsCreation {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	// Needs Update?
	if needsUpdate, err := c.needsUpdate(desiredCR, ctx); needsUpdate || err != nil {
		return managed.ExternalObservation{
			ResourceExists:    true,
			ResourceUpToDate:  !needsUpdate,
			ConnectionDetails: managed.ConnectionDetails{},
		}, err
	}

	if *desiredCR.Status.AtProvider.Status == subaccountStateOk {
		// All fine. Subaccount Usable
		desiredCR.SetConditions(xpv1.Available())
	}
	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) healExternalName(ctx context.Context, cr *apisv1alpha1.Subaccount) error {
	// Defensive: unit tests exercise `external` directly without wiring up the
	// accounts accessor. In production Connect() always sets it.
	if c.accountsAccessor == nil {
		return nil
	}
	if !recovery.HasCreateBeenAttempted(cr) {
		return nil
	}
	subdomain := cr.Spec.ForProvider.Subdomain
	guid, createdAt, found, err := c.accountsAccessor.SubaccountGuidBySubdomain(ctx, subdomain)
	if err != nil {
		ctrl.Log.Info("external-name recovery lookup failed", "subdomain", subdomain, "error", err.Error())
		c.emit(cr, event.Warning(event.Reason(recovery.EventReasonLookupFailed), err))
		return nil
	}
	if !found {
		return nil
	}

	if !recovery.IsOwnedByCR(cr, createdAt) {
		ctrl.Log.Info("external-name recovery refused: BTP subaccount is outside our Create-attempt window (brownfield)",
			"subdomain", subdomain, "guid", guid,
			"crCreatedAt", cr.GetCreationTimestamp().Time, "btpCreatedAt", createdAt)
		c.emit(cr, event.Warning(
			event.Reason(recovery.EventReasonRefusedBrownfield),
			errors.Errorf(
				"refusing to recover existing BTP subaccount %s: created_at %s is outside the window where our own Create() attempt for this CR could have produced it (brownfield). Set crossplane.io/external-name explicitly to import it (see external-name ADR)",
				guid, createdAt.Format(time.RFC3339))))
		return nil
	}

	meta.SetExternalName(cr, guid)
	if uErr := c.Client.Update(ctx, cr); uErr != nil {
		return errors.Wrap(uErr, errUpdateExternalName)
	}

	ctrl.Log.Info("recovered existing BTP subaccount by external-name", "guid", guid, "subdomain", subdomain)
	c.emit(cr, event.Normal(event.Reason(recovery.EventReasonRecovered),
		fmt.Sprintf("Recovered existing BTP subaccount %s (semantic key: subdomain=%s, created_at=%s)", guid, subdomain, createdAt.Format(time.RFC3339))))
	return recovery.ErrRequeueAfterRecovery
}

// emit records a Kubernetes event when a recorder is configured.
func (c *external) emit(cr resource.Managed, ev event.Event) {
	if c.recorder != nil {
		c.recorder.Event(cr, ev)
	}
}

func (c *external) generateObservation(
	ctx context.Context,
	desiredState *apisv1alpha1.Subaccount,
) error {

	subaccount, _, err := c.btp.
		AccountsServiceClient.
		SubaccountOperationsAPI.
		GetSubaccount(ctx, meta.GetExternalName(desiredState)).
		Execute()

	if err != nil {
		resetRemoteState(desiredState)
		if strings.Contains(err.Error(), "404") {
			return nil
		}
		return err
	}
	if subaccount == nil {
		resetRemoteState(desiredState)
		return nil
	}

	desiredState.Status.AtProvider.SubaccountGuid = &subaccount.Guid
	desiredState.Status.AtProvider.Status = &subaccount.State
	desiredState.Status.AtProvider.StatusMessage = subaccount.StateMessage
	desiredState.Status.AtProvider.BetaEnabled = &subaccount.BetaEnabled
	desiredState.Status.AtProvider.Labels = subaccount.Labels
	desiredState.Status.AtProvider.Description = &subaccount.Description
	desiredState.Status.AtProvider.Subdomain = &subaccount.Subdomain
	desiredState.Status.AtProvider.DisplayName = &subaccount.DisplayName
	desiredState.Status.AtProvider.Region = &subaccount.Region
	desiredState.Status.AtProvider.UsedForProduction = &subaccount.UsedForProduction
	desiredState.Status.AtProvider.ParentGuid = &subaccount.ParentGUID
	desiredState.Status.AtProvider.GlobalAccountGUID = &subaccount.GlobalAccountGUID

	return nil
}

func resetRemoteState(state *apisv1alpha1.Subaccount) {
	state.Status.AtProvider = apisv1alpha1.SubaccountObservation{}
}

func (c *external) needsCreation(cr *apisv1alpha1.Subaccount) bool {
	if cr.Status.AtProvider.SubaccountGuid == nil {
		return true
	}

	if cr.Status.AtProvider.Status == nil {
		return true
	}

	return false
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*apisv1alpha1.Subaccount)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotSubaccount)
	}

	if cr.Status.AtProvider.Status != nil && *cr.Status.AtProvider.Status == "STARTED" {
		return managed.ExternalCreation{}, nil
	}

	err := c.createBTPSubaccount(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}
	cr.SetConditions(xpv1.Creating())

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) needsUpdate(cr *apisv1alpha1.Subaccount, ctx context.Context) (bool, error) {
	if needsUpdate(cr.Spec, cr.Status) {
		return true, nil
	}
	return false, nil
}

func needsUpdate(desired apisv1alpha1.SubaccountSpec, actual apisv1alpha1.SubaccountStatus) bool {
	cleanedDesired := desired.ForProvider.DeepCopy()
	cleanedActual := actual.AtProvider.DeepCopy()
	// Remove non-diff relevant information

	filter(cleanedActual.Labels, apisv1alpha1.SubaccountOperatorLabel)
	cleanedDesired.SubaccountAdmins = nil

	if cleanedDesired.Description == "" && cleanedActual.Description == nil {
		cleanedActual.Description = internal.Ptr("")
	}

	if !reflect.DeepEqual(&cleanedDesired.Description, cleanedActual.Description) {
		return true
	}
	if !reflect.DeepEqual(&cleanedDesired.DisplayName, cleanedActual.DisplayName) {
		return true
	}
	if !reflect.DeepEqual(&cleanedDesired.UsedForProduction, cleanedActual.UsedForProduction) {
		return true
	}
	if changedLabels(cleanedDesired.Labels, cleanedActual.Labels) {
		return true
	}
	if !reflect.DeepEqual(&cleanedDesired.BetaEnabled, cleanedActual.BetaEnabled) {
		return true
	}
	if directoryParentChanged(cleanedDesired, cleanedActual) {
		return true
	}
	return false
}

func filter(labels *map[string][]string, toRemove string) map[string][]string {
	var resultLabels map[string][]string
	if labels != nil {
		resultLabels = *labels
		delete(resultLabels, toRemove)
	}
	return resultLabels
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*apisv1alpha1.Subaccount)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotSubaccount)
	}

	if cr.Status.AtProvider.Status != nil && *cr.Status.AtProvider.Status == "CREATING" {
		return managed.ExternalUpdate{}, nil
	}

	subaccount := cr
	connectionDetails := managed.ConnectionDetails{}

	if err := c.updateBTPSubaccount(ctx, subaccount); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	return managed.ExternalUpdate{
		ConnectionDetails: connectionDetails,
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*apisv1alpha1.Subaccount)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotSubaccount)
	}

	c.tracker.SetConditions(ctx, cr)
	if blocked := c.tracker.DeleteShouldBeBlocked(mg); blocked {
		return managed.ExternalDelete{}, errors.New(providerv1alpha1.ErrResourceInUse)
	}

	if cr.Status.AtProvider.Status != nil && *cr.Status.AtProvider.Status == subaccountStateDeleting {
		return managed.ExternalDelete{}, nil
	}

	cr.SetConditions(xpv1.Deleting())

	subaccount := cr

	return managed.ExternalDelete{}, errors.Wrap(c.deleteBTPSubaccount(ctx, subaccount), errDelete)
}

func (c *external) deleteBTPSubaccount(
	ctx context.Context,
	subaccount *apisv1alpha1.Subaccount,
) error {
	subaccount.SetConditions(xpv1.Deleting())

	subaccountId := meta.GetExternalName(subaccount)

	_, raw, err := c.btp.AccountsServiceClient.SubaccountOperationsAPI.DeleteSubaccount(ctx, subaccountId).Execute()
	// 404 not found means already deleted - not considered as error case
	if raw != nil && raw.StatusCode == 404 {
		ctrl.Log.Info("associated BTP subaccount not found, continue deletion")
		return nil
	}

	if err != nil {
		aErr := specifyAPIError(err)
		if code, ok := apiErrorCode(aErr); ok && code == btpErrCodeActiveServiceInstances {
			return errors.Wrap(c.describeDeletionBlockers(ctx, subaccountId, aErr), "deletion of subaccount failed")
		}
		return errors.Wrap(aErr, "deletion of subaccount failed")
	}

	return nil
}

// describeDeletionBlockers enriches a code-70011 rejection by naming the
// service instances that block the deletion. BTP reports only that active
// instances exist, which is a dead end when those instances have no managed
// resource of their own.
//
// The lookup is read-only and best-effort. When it cannot be performed the
// original error is returned carrying the subaccount GUID and a precise manual
// next step instead — plus, whenever the provider actually has one, the reason
// the enumeration did not succeed. Reporting every failure as "no admin binding
// exists" would be an actively misleading diagnosis on a path whose entire
// purpose is actionability: an expired binding, a denied scope and a transport
// error each need a different response from the operator.
func (c *external) describeDeletionBlockers(ctx context.Context, subaccountGuid string, cause error) error {
	// No lister wired at all, or none obtainable: the normal case for
	// subaccounts this provider never created a service-manager instance in.
	noBinding := func(reason error) error {
		why := "no service-manager admin binding is available in this subaccount"
		if reason != nil {
			why = fmt.Sprintf("no service-manager admin binding could be obtained: %v", reason)
		}
		return errors.New(deletionBlockerHint(cause, subaccountGuid, why))
	}

	if c.newInstanceListerFn == nil {
		return noBinding(nil)
	}
	lister, lErr := c.newInstanceListerFn(ctx, subaccountGuid)
	if lErr != nil {
		return noBinding(lErr)
	}
	if lister == nil {
		return noBinding(nil)
	}

	// The list call itself failed: the provider knows why (ListServiceInstances
	// surfaces BTP's own description), so say so rather than blaming a missing
	// binding that does exist.
	instances, lErr := lister.ListServiceInstances(ctx)
	if lErr != nil {
		return errors.New(deletionBlockerHint(cause, subaccountGuid,
			fmt.Sprintf("the provider could not enumerate them: %v", lErr)))
	}
	if len(instances) == 0 {
		return errors.New(deletionBlockerHint(cause, subaccountGuid,
			"the provider found none, so they are not visible to the service-manager binding it used"))
	}

	return errors.Errorf("%v; subaccount %s still has %d active service instance(s) blocking deletion: %s",
		cause, subaccountGuid, len(instances), formatBlockers(instances, maxListedBlockers))
}

// deletionBlockerHint renders the fallback message for a code-70011 rejection
// the provider could not resolve into a list of instances. why states what
// actually went wrong so the operator is not sent after the wrong cause.
func deletionBlockerHint(cause error, subaccountGuid, why string) string {
	return fmt.Sprintf(
		"%v; subaccount %s still has active service instances, but %s. "+
			"List them with the Service Manager API: GET <sm_url>/v1/service_instances, "+
			"or via the BTP cockpit under Services > Instances and Subscriptions.",
		cause, subaccountGuid, why)
}

// formatBlockers renders at most max entries as `name (id)` and appends
// "... and N more", so the surfaced message cannot grow with the instance
// count.
func formatBlockers(refs []smClient.ServiceInstanceRef, max int) string {
	shown := refs
	if len(shown) > max {
		shown = shown[:max]
	}

	parts := make([]string, 0, len(shown))
	for _, r := range shown {
		parts = append(parts, fmt.Sprintf("%s (%s)", r.Name, r.ID))
	}

	out := strings.Join(parts, ", ")
	if len(refs) > len(shown) {
		out += fmt.Sprintf(", ... and %d more", len(refs)-len(shown))
	}
	return out
}

func (c *external) updateBTPSubaccount(
	ctx context.Context, subaccount *apisv1alpha1.Subaccount,
) error {
	if directoryParentChanged(&subaccount.Spec.ForProvider, &subaccount.Status.AtProvider) {
		return c.moveSubaccountAPI(ctx, subaccount)
	} else {
		return c.updateSubaccountAPI(ctx, subaccount)
	}
}

func (c *external) moveSubaccountAPI(ctx context.Context, subaccount *apisv1alpha1.Subaccount) error {
	guid := meta.GetExternalName(subaccount)

	targetID := subaccount.Spec.ForProvider.DirectoryGuid
	// if not specified we need to set the global account as parent
	if emptyDirectoryRef(&subaccount.Spec.ForProvider) {
		targetID = internal.Val(subaccount.Status.AtProvider.GlobalAccountGUID)
	}

	err := c.accountsAccessor.MoveSubaccount(ctx, guid, targetID)
	if err != nil {
		return errors.Wrap(specifyAPIError(err), errMoveSubaccount)
	}
	return nil
}

func (c *external) updateSubaccountAPI(ctx context.Context, subaccount *apisv1alpha1.Subaccount) error {
	guid := meta.GetExternalName(subaccount)

	label := addOperatorLabel(subaccount)

	params := accountclient.UpdateSubaccountRequestPayload{
		BetaEnabled:       &subaccount.Spec.ForProvider.BetaEnabled,
		Description:       &subaccount.Spec.ForProvider.Description,
		DisplayName:       subaccount.Spec.ForProvider.DisplayName,
		Labels:            &label,
		UsedForProduction: &subaccount.Spec.ForProvider.UsedForProduction,
	}

	err := c.accountsAccessor.UpdateSubaccount(ctx, guid, params)
	if err != nil {
		return errors.Wrap(specifyAPIError(err), errUpdateAPI)
	}
	return nil
}

func (c *external) createBTPSubaccount(
	ctx context.Context, subaccount *apisv1alpha1.Subaccount,
) error {
	ctrl.Log.Info(fmt.Sprintf("Creating subaccount: %s", subaccount.Name))
	createdSubaccount, resp, err := c.btp.AccountsServiceClient.SubaccountOperationsAPI.
		CreateSubaccount(ctx).
		CreateSubaccountRequestPayload(toCreateApiPayload(subaccount)).
		Execute()
	if err != nil {
		// Check if error is "resource already exists"
		if resp != nil && resp.StatusCode == http.StatusConflict {
			// ADR: Do NOT set external-name, stay in error loop
			// User must set external-name manually to resolve
			return errors.Wrap(err, "creation failed - resource already exists. Please set external-name annotation to adopt the existing resource")
		}
		// Other errors: do not set external-name either
		return specifyAPIError(err)
	}

	// ADR: Successful creation - set external-name from API response
	guid := createdSubaccount.Guid
	ctrl.Log.Info(fmt.Sprintf("subaccount (%s) created", guid))
	subaccount.Status.AtProvider.SubaccountGuid = &guid
	subaccount.Status.AtProvider.Status = createdSubaccount.StateMessage
	subaccount.Status.AtProvider.ParentGuid = &createdSubaccount.ParentGUID

	meta.SetExternalName(subaccount, guid)

	return nil
}

// In earlier versions, the name of the subaccoung was used as the external-name.
// Currently it is the id of the subaccount. To be able to upgrade from this
// earlier version, if the external-name value is equal to the name of the
// subaccount, update it to the id.
func (c *external) migrateExternalName(ctx context.Context, subaccount *apisv1alpha1.Subaccount) error {
	externalName := meta.GetExternalName(subaccount)

	// if externalName is already the id, the migration can be skippped
	if isValidUUID(externalName) {
		return nil
	}

	response, _, err := c.btp.AccountsServiceClient.SubaccountOperationsAPI.GetSubaccounts(ctx).Execute()
	if err != nil {
		return errors.Wrap(specifyAPIError(err), errGetSubaccounts)
	}

	btpSubaccounts := response.Value
	for _, account := range btpSubaccounts {
		if isRelatedAccount(subaccount, &account) {
			// update the old externalName to be the uid
			meta.SetExternalName(subaccount, account.Guid)
			if err := c.Client.Update(ctx, subaccount); err != nil {
				return errors.Wrap(err, errUpdateExternalName)
			}
			break
		}
	}

	return nil

}

func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func isRelatedAccount(subaccount *apisv1alpha1.Subaccount, account *accountclient.SubaccountResponseObject) bool {
	return strings.Compare(
		subaccount.Spec.ForProvider.Subdomain, account.Subdomain,
	) == 0 && strings.Compare(subaccount.Spec.ForProvider.Region, account.Region) == 0
}

func toCreateApiPayload(subaccount *apisv1alpha1.Subaccount) accountclient.CreateSubaccountRequestPayload {
	subaccountSpec := subaccount.Spec

	label := addOperatorLabel(subaccount)

	return accountclient.CreateSubaccountRequestPayload{
		BetaEnabled:       &subaccountSpec.ForProvider.BetaEnabled,
		Description:       &subaccountSpec.ForProvider.Description,
		DisplayName:       subaccountSpec.ForProvider.DisplayName,
		Labels:            &label,
		Region:            subaccountSpec.ForProvider.Region,
		SubaccountAdmins:  subaccountSpec.ForProvider.SubaccountAdmins,
		Subdomain:         &subaccountSpec.ForProvider.Subdomain,
		UsedForProduction: &subaccountSpec.ForProvider.UsedForProduction,
		ParentGUID:        &subaccountSpec.ForProvider.DirectoryGuid,
	}
}

func addOperatorLabel(subaccount *apisv1alpha1.Subaccount) map[string][]string {
	if subaccount.Spec.ForProvider.Labels == nil {
		return map[string][]string{}
	}
	labels := map[string][]string{}
	for k, v := range subaccount.Spec.ForProvider.Labels {
		labels[k] = v
	}
	labels[apisv1alpha1.SubaccountOperatorLabel] = []string{string(subaccount.UID)}
	return labels
}

func directoryParentChanged(spec *apisv1alpha1.SubaccountParameters, status *apisv1alpha1.SubaccountObservation) bool {
	supposeGlobal := emptyDirectoryRef(spec)
	// With no directory specified we expect it to be in global account
	if supposeGlobal {
		return !reflect.DeepEqual(status.ParentGuid, status.GlobalAccountGUID)
	}
	return !reflect.DeepEqual(status.ParentGuid, &spec.DirectoryGuid)
}

func emptyDirectoryRef(spec *apisv1alpha1.SubaccountParameters) bool {
	return spec.DirectoryRef == nil && spec.DirectorySelector == nil && spec.DirectoryGuid == ""
}

// apiError carries the BTP error code alongside the message so callers can
// branch on the code instead of matching on the rendered string. Error()
// renders exactly as the previous string-only form did, so nothing that reads
// the message changes.
type apiError struct {
	Code    int
	Message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("API Error: %v, Code %v", e.Message, e.Code)
}

func specifyAPIError(err error) error {
	if genericErr, ok := err.(*accountclient.GenericOpenAPIError); ok {
		if accountError, ok := genericErr.Model().(accountclient.ApiExceptionResponseObject); ok {
			return &apiError{
				Code:    int(internal.Val(accountError.Error.Code)),
				Message: internal.Val(accountError.Error.Message),
			}
		}
		if genericErr.Body() != nil {
			return fmt.Errorf("API Error: %s", string(genericErr.Body()))
		}
	}
	return err
}

// apiErrorCode reports the BTP error code carried by err, if any.
func apiErrorCode(err error) (int, bool) {
	var aErr *apiError
	if errors.As(err, &aErr) {
		return aErr.Code, true
	}
	return 0, false
}

func changedLabels(specLabels map[string]apisv1alpha1.SubaccountLabelValueList, statusLabels *map[string][]string) bool {
	// pointer to maps can be pointer to nil values, which won't deep equal as expected here, so we need to treat this case manually
	if statusLabels == nil {
		return len(specLabels) != 0
	}
	if len(*statusLabels) == 0 && len(specLabels) == 0 {
		return false
	}
	if len(specLabels) != len(*statusLabels) {
		return true
	}
	for k, v := range specLabels {
		if !reflect.DeepEqual([]string(v), (*statusLabels)[k]) {
			return true
		}
	}
	return false
}
