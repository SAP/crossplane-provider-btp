package subaccount

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotSubaccount        = "managed resource is not a Subaccount custom resource"
	errSubaccountNotFound   = "subaccount not found"
	subaccountStateDeleting = "DELETING"
	subaccountStateOk       = "OK"
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	usage           resource.Tracker
	resourcetracker tracking.ReferenceResolverTracker

	newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)
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
		return nil, err
	}

	return &external{
		Client:           c.kube,
		btp:              *btpclient,
		tracker:          c.resourcetracker,
		accountsAccessor: &AccountsClient{btp: *btpclient},
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
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	desiredCR, ok := mg.(*apisv1alpha1.Subaccount)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotSubaccount)
	}

	if err := c.generateObservation(ctx, desiredCR); err != nil {
		return managed.ExternalObservation{}, err
	}

	c.tracker.SetConditions(ctx, desiredCR)
	// Needs Creation?
	if needsCreation := c.needsCreation(desiredCR); needsCreation {
		return managed.ExternalObservation{
			ResourceExists: !needsCreation,
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

func (c *external) generateObservation(
	ctx context.Context,
	desiredState *apisv1alpha1.Subaccount,
) error {
	subaccount, err := c.findBTPSubaccount(ctx, desiredState)
	if err != nil {
		resetRemoteState(desiredState)
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
		return managed.ExternalCreation{}, err
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
		return managed.ExternalUpdate{}, err
	}

	return managed.ExternalUpdate{
		ConnectionDetails: connectionDetails,
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*apisv1alpha1.Subaccount)
	if !ok {
		return errors.New(errNotSubaccount)
	}

	c.tracker.SetConditions(ctx, cr)
	if blocked := c.tracker.DeleteShouldBeBlocked(mg); blocked {
		return errors.New(providerv1alpha1.ErrResourceInUse)
	}

	if cr.Status.AtProvider.Status != nil && *cr.Status.AtProvider.Status == subaccountStateDeleting {
		return nil
	}

	cr.SetConditions(xpv1.Deleting())

	subaccount := cr

	return deleteBTPSubaccount(ctx, subaccount, c.btp)
}

func deleteBTPSubaccount(
	ctx context.Context,
	subaccount *apisv1alpha1.Subaccount,
	accountsServiceClient btp.Client,
) error {
	subaccount.SetConditions(xpv1.Deleting())

	subaccountId := *subaccount.Status.AtProvider.SubaccountGuid

	response, raw, err := accountsServiceClient.AccountsServiceClient.SubaccountOperationsAPI.DeleteSubaccount(ctx, subaccountId).Execute()
	if raw.StatusCode == 404 {
		ctrl.Log.Info("associated BTP subaccount not found, continue deletion")
		return nil
	}

	if err != nil {
		return errors.Wrap(err, "deletion of subaccount failed")
	}

	deletionState := response.State
	return errors.New(fmt.Sprintf("Deletion Pending: Current status: %s", deletionState))
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
	guid := subaccount.Status.AtProvider.SubaccountGuid
	targetID := subaccount.Spec.ForProvider.DirectoryGuid
	// if not specified we need to set the global account as parent
	if emptyDirectoryRef(&subaccount.Spec.ForProvider) {
		targetID = internal.Val(subaccount.Status.AtProvider.GlobalAccountGUID)
	}

	err := c.accountsAccessor.MoveSubaccount(ctx, internal.Val(guid), targetID)
	if err != nil {
		return errors.Wrap(err, "moving subaccount failed")
	}
	return nil
}

func (c *external) updateSubaccountAPI(ctx context.Context, subaccount *apisv1alpha1.Subaccount) error {
	guid := subaccount.Status.AtProvider.SubaccountGuid

	label := addOperatorLabel(subaccount)

	params := accountclient.UpdateSubaccountRequestPayload{
		BetaEnabled:       &subaccount.Spec.ForProvider.BetaEnabled,
		Description:       &subaccount.Spec.ForProvider.Description,
		DisplayName:       subaccount.Spec.ForProvider.DisplayName,
		Labels:            &label,
		UsedForProduction: &subaccount.Spec.ForProvider.UsedForProduction,
	}

	err := c.accountsAccessor.UpdateSubaccount(ctx, internal.Val(guid), params)
	if err != nil {
		return errors.Wrap(err, "update of subaccount failed")
	}
	return nil
}

func (c *external) createBTPSubaccount(
	ctx context.Context, subaccount *apisv1alpha1.Subaccount,
) error {
	ctrl.Log.Info(fmt.Sprintf("Creating subaccount: %s", subaccount.Name))
	createdSubaccount, _, err := c.btp.AccountsServiceClient.SubaccountOperationsAPI.
		CreateSubaccount(ctx).
		CreateSubaccountRequestPayload(toCreateApiPayload(subaccount)).
		Execute()
	if err != nil {
		return specifyAPIError(err)
	}

	guid := createdSubaccount.Guid
	ctrl.Log.Info(fmt.Sprintf("subaccount (%s) created", guid))
	subaccount.Status.AtProvider.SubaccountGuid = &guid
	subaccount.Status.AtProvider.Status = createdSubaccount.StateMessage
	subaccount.Status.AtProvider.ParentGuid = &createdSubaccount.ParentGUID

	return nil
}

func (c *external) findBTPSubaccount(
	ctx context.Context, subaccount *apisv1alpha1.Subaccount,
) (*accountclient.SubaccountResponseObject, error) {
	response, _, err := c.btp.AccountsServiceClient.SubaccountOperationsAPI.GetSubaccounts(ctx).Execute()
	if err != nil {
		ctrl.Log.Error(err, "could not get BTP subaccounts")
		return nil, err
	}

	var foundAccount *accountclient.SubaccountResponseObject = nil
	btpSubaccounts := response.Value
	for _, account := range btpSubaccounts {
		if isRelatedAccount(subaccount, &account) {
			foundAccount = &account
			break
		}
	}

	return foundAccount, nil
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
	internal.CopyMaps(labels, subaccount.Spec.ForProvider.Labels)
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

func specifyAPIError(err error) error {
	if genericErr, ok := err.(*accountclient.GenericOpenAPIError); ok {
		if accountError, ok := genericErr.Model().(accountclient.ApiExceptionResponseObject); ok {
			return errors.New(fmt.Sprintf("API Error: %v, Code %v", internal.Val(accountError.Error.Message), internal.Val(accountError.Error.Code)))
		}
		if genericErr.Body() != nil {
			return fmt.Errorf("API Error: %s", string(genericErr.Body()))
		}
	}
	return err
}

func changedLabels(specLabels map[string][]string, statusLabels *map[string][]string) bool {
	// pointer to maps can be pointer to nil values, which won't deep equal as expected here, so we need to treat this case manually
	if statusLabels == nil {
		return len(specLabels) != 0
	}
	if len(*statusLabels) == 0 && len(specLabels) == 0 {
		return false
	}
	return !reflect.DeepEqual(specLabels, *statusLabels)
}
