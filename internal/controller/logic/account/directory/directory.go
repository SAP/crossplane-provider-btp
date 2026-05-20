package directory

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"

	base "github.com/sap/crossplane-provider-btp/apis/base/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/controller/logic"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

// Options is the per-Setup configuration; aliased to the shared logic.Options so the
// generated controller can refer to it as logic.Options uniformly across resources.
type Options = logic.Options

// Client defines the interface for directory operations.
type Client struct {
	btp     btp.Client
	tracker tracking.ReferenceResolverTracker
}

// Setup wires up the Directory controller via the shared logic.Setup helper.
func Setup(mgr ctrl.Manager, o Options, obj client.Object, kind string, gvk schema.GroupVersionKind, mk logic.MakeExternal) error {
	return logic.Setup(mgr, o, obj, kind, gvk, mk)
}

// Connect builds a *btp.Client for the supplied managed resource and wraps it for
// per-call use by the generated controller. Returns the tracker alongside so the
// returned Client can SetConditions / DeleteShouldBeBlocked from Observe and Delete.
func Connect(ctx context.Context, mg resource.Managed, kube client.Client) (Client, error) {
	btpClient, tracker, err := logic.BuildBTPClient(ctx, mg, kube)
	if err != nil {
		return Client{}, err
	}
	return Client{btp: *btpClient, tracker: tracker}, nil
}

// MigrateExternalName is a no-op for Directory (no legacy format migration needed).
func MigrateExternalName(_ Client, _ context.Context, _ resource.Managed, _ *base.BaseDirectory, _ client.Client) error {
	return nil
}

// Observe checks the external state of a directory.
func Observe(c Client, ctx context.Context, mg resource.Managed, cr *base.BaseDirectory) (managed.ExternalObservation, error) {
	if c.tracker != nil {
		c.tracker.SetConditions(ctx, mg)
	}

	if meta.GetExternalName(cr) == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	if !isValidUUID(meta.GetExternalName(cr)) {
		return managed.ExternalObservation{}, fmt.Errorf("external-name '%s' is not a valid GUID", meta.GetExternalName(cr))
	}

	directory, err := getDirectory(c, ctx, cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	if directory == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	cr.Status.AtProvider.Guid = &directory.Guid
	cr.Status.AtProvider.EntityState = directory.EntityState
	cr.Status.AtProvider.StateMessage = directory.StateMessage
	cr.Status.AtProvider.Subdomain = directory.Subdomain
	cr.Status.AtProvider.DirectoryFeatures = directory.DirectoryFeatures

	if cr.Status.AtProvider.EntityState != nil && *cr.Status.AtProvider.EntityState != base.DirectoryEntityStateOk {
		cr.Status.SetConditions(xpv1.Unavailable())
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: true,
		}, nil
	}

	cr.Status.SetConditions(xpv1.Available())

	if !isSynced(cr, directory) {
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: false,
		}, nil
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true,
	}, nil
}

// Create provisions a new directory.
func Create(c Client, ctx context.Context, mg resource.Managed, cr *base.BaseDirectory) (managed.ExternalCreation, error) {
	cr.Status.SetConditions(xpv1.Creating())

	var displayName string
	if cr.Spec.ForProvider.DisplayName != nil {
		displayName = *cr.Spec.ForProvider.DisplayName
	}

	payload := accountclient.CreateDirectoryRequestPayload{
		Description:       cr.Spec.ForProvider.Description,
		DirectoryAdmins:   cr.Spec.ForProvider.DirectoryAdmins,
		DirectoryFeatures: cr.Spec.ForProvider.DirectoryFeatures,
		DisplayName:       displayName,
		Labels:            &cr.Spec.ForProvider.Labels,
		Subdomain:         cr.Spec.ForProvider.Subdomain,
	}

	directory, resp, err := c.btp.AccountsServiceClient.DirectoryOperationsAPI.
		CreateDirectory(ctx).
		ParentGUID(cr.Spec.ForProvider.DirectoryGuid).
		CreateDirectoryRequestPayload(payload).
		Execute()

	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusConflict {
			return managed.ExternalCreation{}, fmt.Errorf("creation failed - directory already exists. Please set external-name annotation to adopt the existing resource: %w", specifyAPIError(err))
		}
		return managed.ExternalCreation{}, specifyAPIError(err)
	}

	guid := directory.Guid
	ctrl.Log.Info(fmt.Sprintf("directory (%s) created", guid))
	meta.SetExternalName(cr, guid)

	return managed.ExternalCreation{}, nil
}

// Update modifies an existing directory.
func Update(c Client, ctx context.Context, mg resource.Managed, cr *base.BaseDirectory) (managed.ExternalUpdate, error) {
	extID := meta.GetExternalName(cr)
	if extID == "" {
		return managed.ExternalUpdate{}, errors.New("can not request API without GUID")
	}

	params := accountclient.UpdateDirectoryRequestPayload{
		Description: cr.Spec.ForProvider.Description,
		DisplayName: cr.Spec.ForProvider.DisplayName,
		Labels:      &cr.Spec.ForProvider.Labels,
	}

	_, _, err := c.btp.AccountsServiceClient.DirectoryOperationsAPI.
		UpdateDirectory(ctx, extID).
		UpdateDirectoryRequestPayload(params).
		Execute()
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	featuresPayload := accountclient.UpdateDirectoryTypeRequestPayload{
		DirectoryFeatures: cr.Spec.ForProvider.DirectoryFeatures,
		DirectoryAdmins:   cr.Spec.ForProvider.DirectoryAdmins,
		Subdomain:         cr.Spec.ForProvider.Subdomain,
	}

	_, _, err = c.btp.AccountsServiceClient.DirectoryOperationsAPI.
		UpdateDirectoryFeatures(ctx, extID).
		UpdateDirectoryTypeRequestPayload(featuresPayload).
		Execute()

	return managed.ExternalUpdate{}, err
}

// Delete removes a directory.
func Delete(c Client, ctx context.Context, mg resource.Managed, cr *base.BaseDirectory) (managed.ExternalDelete, error) {
	if c.tracker != nil {
		c.tracker.SetConditions(ctx, mg)
		if c.tracker.DeleteShouldBeBlocked(mg) {
			return managed.ExternalDelete{}, logic.DeleteBlockedError()
		}
	}

	if cr.Status.AtProvider.EntityState != nil && *cr.Status.AtProvider.EntityState == "DELETING" {
		return managed.ExternalDelete{}, nil
	}

	cr.Status.SetConditions(xpv1.Deleting())

	extID := meta.GetExternalName(cr)
	if extID == "" {
		return managed.ExternalDelete{}, errors.New("can not request API without GUID")
	}

	_, resp, err := c.btp.AccountsServiceClient.DirectoryOperationsAPI.DeleteDirectory(ctx, extID).Execute()
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		ctrl.Log.Info("associated BTP directory not found, continue deletion")
		return managed.ExternalDelete{}, nil
	}

	if err != nil {
		return managed.ExternalDelete{}, err
	}

	return managed.ExternalDelete{}, nil
}

func getDirectory(c Client, ctx context.Context, cr *base.BaseDirectory) (*accountclient.DirectoryResponseObject, error) {
	extID := meta.GetExternalName(cr)

	directory, raw, err := c.btp.AccountsServiceClient.DirectoryOperationsAPI.GetDirectory(ctx, extID).Execute()
	if raw != nil && raw.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, specifyAPIError(err)
	}
	return directory, nil
}

func isSynced(cr *base.BaseDirectory, api *accountclient.DirectoryResponseObject) bool {
	providedDirectoryFeatures := cr.Spec.ForProvider.DirectoryFeatures
	if providedDirectoryFeatures == nil {
		providedDirectoryFeatures = []string{"DEFAULT"}
	}

	return internal.Val(cr.Spec.ForProvider.Description) == internal.Val(api.Description) &&
		internal.Val(cr.Spec.ForProvider.DisplayName) == api.DisplayName &&
		reflect.DeepEqual(cr.Spec.ForProvider.Labels, internal.Val(api.Labels)) &&
		reflect.DeepEqual(providedDirectoryFeatures, api.DirectoryFeatures)
}

func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func specifyAPIError(err error) error {
	if genericErr, ok := err.(*accountclient.GenericOpenAPIError); ok {
		if accountError, ok := genericErr.Model().(accountclient.ApiExceptionResponseObject); ok {
			return fmt.Errorf("API Error: %v, Code %v", internal.Val(accountError.Error.Message), internal.Val(accountError.Error.Code))
		}
		if genericErr.Body() != nil {
			return fmt.Errorf("API Error: %s", string(genericErr.Body()))
		}
	}
	return err
}
