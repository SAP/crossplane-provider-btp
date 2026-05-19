package subaccount

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	base "github.com/sap/crossplane-provider-btp/apis/base/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

const (
	subaccountStateDeleting = "DELETING"
	subaccountStateOk       = "OK"
)

// Client defines the interface for subaccount operations.
type Client struct {
	btp btp.Client
}

// Connect creates a Client from a BTP client.
func Connect(btpClient *btp.Client, _ client.Client) Client {
	return Client{btp: *btpClient}
}

// MigrateExternalName migrates legacy external-name format (resource name) to GUID.
// In earlier versions, the subaccount name was used as external-name. This function
// detects non-UUID external-names and resolves the correct GUID by matching on
// subdomain+region from the BTP API.
func MigrateExternalName(c Client, ctx context.Context, mg resource.Managed, baseCR *base.BaseSubaccount, kube client.Client) error {
	externalName := meta.GetExternalName(mg)
	if externalName == "" || isValidUUID(externalName) {
		return nil
	}

	response, _, err := c.btp.AccountsServiceClient.SubaccountOperationsAPI.GetSubaccounts(ctx).Execute()
	if err != nil {
		return errors.Wrap(err, "while getting subaccounts")
	}

	for _, account := range response.Value {
		if account.Subdomain == baseCR.Spec.ForProvider.Subdomain && account.Region == baseCR.Spec.ForProvider.Region {
			meta.SetExternalName(mg, account.Guid)
			if err := kube.Update(ctx, mg); err != nil {
				return errors.Wrap(err, "while updating external name")
			}
			break
		}
	}
	return nil
}

// Observe checks the external state of a subaccount.
func Observe(c Client, ctx context.Context, cr *base.BaseSubaccount) (managed.ExternalObservation, error) {
	if meta.GetExternalName(cr) == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	if !isValidUUID(meta.GetExternalName(cr)) {
		return managed.ExternalObservation{}, errors.New(fmt.Sprintf("external-name '%s' is not a valid GUID", meta.GetExternalName(cr)))
	}

	if err := generateObservation(c, ctx, cr); err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "while generating observation")
	}

	if needsCreation(cr) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	if needsUpdate(cr) {
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: false,
		}, nil
	}

	if cr.Status.AtProvider.Status != nil && *cr.Status.AtProvider.Status == subaccountStateOk {
		cr.Status.SetConditions(xpv1.Available())
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true,
	}, nil
}

// Create provisions a new subaccount.
func Create(c Client, ctx context.Context, cr *base.BaseSubaccount) (managed.ExternalCreation, error) {
	if cr.Status.AtProvider.Status != nil && *cr.Status.AtProvider.Status == "STARTED" {
		return managed.ExternalCreation{}, nil
	}

	if err := createBTPSubaccount(c, ctx, cr); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "while creating subaccount")
	}

	cr.Status.SetConditions(xpv1.Creating())
	return managed.ExternalCreation{}, nil
}

// Update modifies an existing subaccount.
func Update(c Client, ctx context.Context, cr *base.BaseSubaccount) (managed.ExternalUpdate, error) {
	if cr.Status.AtProvider.Status != nil && *cr.Status.AtProvider.Status == "CREATING" {
		return managed.ExternalUpdate{}, nil
	}

	if err := updateBTPSubaccount(c, ctx, cr); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "while updating subaccount")
	}

	return managed.ExternalUpdate{}, nil
}

// Delete removes a subaccount.
func Delete(c Client, ctx context.Context, cr *base.BaseSubaccount) (managed.ExternalDelete, error) {
	if cr.Status.AtProvider.Status != nil && *cr.Status.AtProvider.Status == subaccountStateDeleting {
		return managed.ExternalDelete{}, nil
	}

	cr.Status.SetConditions(xpv1.Deleting())

	subaccountId := meta.GetExternalName(cr)
	_, raw, err := c.btp.AccountsServiceClient.SubaccountOperationsAPI.DeleteSubaccount(ctx, subaccountId).Execute()
	if raw != nil && raw.StatusCode == http.StatusNotFound {
		ctrl.Log.Info("associated BTP subaccount not found, continue deletion")
		return managed.ExternalDelete{}, nil
	}
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "while deleting subaccount")
	}

	return managed.ExternalDelete{}, nil
}

func generateObservation(c Client, ctx context.Context, cr *base.BaseSubaccount) error {
	subaccount, _, err := c.btp.AccountsServiceClient.SubaccountOperationsAPI.
		GetSubaccount(ctx, meta.GetExternalName(cr)).Execute()

	if err != nil {
		resetRemoteState(cr)
		if strings.Contains(err.Error(), "404") {
			return nil
		}
		return err
	}
	if subaccount == nil {
		resetRemoteState(cr)
		return nil
	}

	cr.Status.AtProvider.SubaccountGuid = &subaccount.Guid
	cr.Status.AtProvider.Status = &subaccount.State
	cr.Status.AtProvider.StatusMessage = subaccount.StateMessage
	cr.Status.AtProvider.BetaEnabled = &subaccount.BetaEnabled
	cr.Status.AtProvider.Labels = subaccount.Labels
	cr.Status.AtProvider.Description = &subaccount.Description
	cr.Status.AtProvider.Subdomain = &subaccount.Subdomain
	cr.Status.AtProvider.DisplayName = &subaccount.DisplayName
	cr.Status.AtProvider.Region = &subaccount.Region
	cr.Status.AtProvider.UsedForProduction = &subaccount.UsedForProduction
	cr.Status.AtProvider.ParentGuid = &subaccount.ParentGUID
	cr.Status.AtProvider.GlobalAccountGUID = &subaccount.GlobalAccountGUID

	return nil
}

func resetRemoteState(cr *base.BaseSubaccount) {
	cr.Status.AtProvider = base.BaseSubaccountObservation{}
}

func needsCreation(cr *base.BaseSubaccount) bool {
	return cr.Status.AtProvider.SubaccountGuid == nil || cr.Status.AtProvider.Status == nil
}

func needsUpdate(cr *base.BaseSubaccount) bool {
	spec := cr.Spec.ForProvider.DeepCopy()
	status := cr.Status.AtProvider.DeepCopy()

	filter(status.Labels, base.SubaccountOperatorLabel)
	spec.SubaccountAdmins = nil

	if spec.Description == "" && status.Description == nil {
		status.Description = internal.Ptr("")
	}

	if !reflect.DeepEqual(&spec.Description, status.Description) {
		return true
	}
	if !reflect.DeepEqual(&spec.DisplayName, status.DisplayName) {
		return true
	}
	if !reflect.DeepEqual(&spec.UsedForProduction, status.UsedForProduction) {
		return true
	}
	if changedLabels(spec.Labels, status.Labels) {
		return true
	}
	if !reflect.DeepEqual(&spec.BetaEnabled, status.BetaEnabled) {
		return true
	}
	if directoryParentChanged(cr.Spec.ForProvider.DirectoryGuid, status) {
		return true
	}
	return false
}

func filter(labels *map[string][]string, toRemove string) {
	if labels != nil {
		delete(*labels, toRemove)
	}
}

func createBTPSubaccount(c Client, ctx context.Context, cr *base.BaseSubaccount) error {
	ctrl.Log.Info(fmt.Sprintf("Creating subaccount: %s", cr.Name))

	label := addOperatorLabel(cr)
	payload := accountclient.CreateSubaccountRequestPayload{
		BetaEnabled:       &cr.Spec.ForProvider.BetaEnabled,
		Description:       &cr.Spec.ForProvider.Description,
		DisplayName:       cr.Spec.ForProvider.DisplayName,
		Labels:            &label,
		Region:            cr.Spec.ForProvider.Region,
		SubaccountAdmins:  cr.Spec.ForProvider.SubaccountAdmins,
		Subdomain:         &cr.Spec.ForProvider.Subdomain,
		UsedForProduction: &cr.Spec.ForProvider.UsedForProduction,
		ParentGUID:        &cr.Spec.ForProvider.DirectoryGuid,
	}

	createdSubaccount, resp, err := c.btp.AccountsServiceClient.SubaccountOperationsAPI.
		CreateSubaccount(ctx).
		CreateSubaccountRequestPayload(payload).
		Execute()

	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusConflict {
			return errors.Wrap(err, "creation failed - resource already exists. Please set external-name annotation to adopt the existing resource")
		}
		return specifyAPIError(err)
	}

	guid := createdSubaccount.Guid
	ctrl.Log.Info(fmt.Sprintf("subaccount (%s) created", guid))
	cr.Status.AtProvider.SubaccountGuid = &guid
	cr.Status.AtProvider.Status = createdSubaccount.StateMessage
	cr.Status.AtProvider.ParentGuid = &createdSubaccount.ParentGUID

	meta.SetExternalName(cr, guid)
	return nil
}

func updateBTPSubaccount(c Client, ctx context.Context, cr *base.BaseSubaccount) error {
	status := &cr.Status.AtProvider

	if directoryParentChanged(cr.Spec.ForProvider.DirectoryGuid, status) {
		return moveSubaccountAPI(c, ctx, cr)
	}
	return updateSubaccountAPI(c, ctx, cr)
}

func moveSubaccountAPI(c Client, ctx context.Context, cr *base.BaseSubaccount) error {
	guid := meta.GetExternalName(cr)
	targetID := cr.Spec.ForProvider.DirectoryGuid
	if cr.Spec.ForProvider.DirectoryGuid == "" {
		targetID = internal.Val(cr.Status.AtProvider.GlobalAccountGUID)
	}

	if targetID == "" {
		return errors.New("targetId must be set for move subaccount api call")
	}

	_, _, err := c.btp.AccountsServiceClient.SubaccountOperationsAPI.
		MoveSubaccount(ctx, guid).
		MoveSubaccountRequestPayload(accountclient.MoveSubaccountRequestPayload{
			TargetAccountGUID: targetID,
		}).Execute()

	return err
}

func updateSubaccountAPI(c Client, ctx context.Context, cr *base.BaseSubaccount) error {
	guid := meta.GetExternalName(cr)
	label := addOperatorLabel(cr)

	params := accountclient.UpdateSubaccountRequestPayload{
		BetaEnabled:       &cr.Spec.ForProvider.BetaEnabled,
		Description:       &cr.Spec.ForProvider.Description,
		DisplayName:       cr.Spec.ForProvider.DisplayName,
		Labels:            &label,
		UsedForProduction: &cr.Spec.ForProvider.UsedForProduction,
	}

	_, _, err := c.btp.AccountsServiceClient.SubaccountOperationsAPI.
		UpdateSubaccount(ctx, guid).
		UpdateSubaccountRequestPayload(params).
		Execute()

	return err
}

func addOperatorLabel(cr *base.BaseSubaccount) map[string][]string {
	if cr.Spec.ForProvider.Labels == nil {
		return map[string][]string{}
	}
	labels := map[string][]string{}
	internal.CopyMaps(labels, cr.Spec.ForProvider.Labels)
	labels[base.SubaccountOperatorLabel] = []string{string(cr.UID)}
	return labels
}

func directoryParentChanged(directoryGuid string, status *base.BaseSubaccountObservation) bool {
	if directoryGuid == "" {
		return !reflect.DeepEqual(status.ParentGuid, status.GlobalAccountGUID)
	}
	return !reflect.DeepEqual(status.ParentGuid, &directoryGuid)
}

func changedLabels(specLabels map[string][]string, statusLabels *map[string][]string) bool {
	if statusLabels == nil {
		return len(specLabels) != 0
	}
	if len(*statusLabels) == 0 && len(specLabels) == 0 {
		return false
	}
	return !reflect.DeepEqual(specLabels, *statusLabels)
}

func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
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
