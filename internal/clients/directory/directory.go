package directory

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/google/uuid"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

const errMisUse = "can not request API without GUID"

// DirectoryClientI acts as clear interface between controller and buisness logic
type DirectoryClientI interface {
	CreateDirectory(ctx context.Context) (*v1alpha1.Directory, error)
	UpdateDirectory(ctx context.Context) (*v1alpha1.Directory, error)
	DeleteDirectory(ctx context.Context) error
	NeedsCreation(ctx context.Context) (bool, error)
	NeedsUpdate(ctx context.Context) (bool, error)
	SyncStatus(ctx context.Context) error
	IsAvailable() bool
}

func NewDirectoryClient(btpClient *btp.Client, cr *v1alpha1.Directory) *DirectoryClient {
	return &DirectoryClient{
		btpClient: btpClient,
		cr:        cr,
	}
}

type DirectoryClient struct {
	btpClient *btp.Client
	cr        *v1alpha1.Directory

	cachedApi *accountclient.DirectoryResponseObject
}

func (d *DirectoryClient) UpdateDirectory(ctx context.Context) (*v1alpha1.Directory, error) {
	// without an externalID we can't connect to the API
	if d.externalID() == "" {
		return d.cr, errors.New(errMisUse)
	}

	params := d.toUpdateApiPayload()

	_, _, err := d.btpClient.AccountsServiceClient.DirectoryOperationsAPI.
		UpdateDirectory(ctx, d.externalID()).
		UpdateDirectoryRequestPayload(params).
		Execute()

	if err != nil {
		return d.cr, err
	}

	_, _, err = d.btpClient.AccountsServiceClient.DirectoryOperationsAPI.
		UpdateDirectoryFeatures(ctx, d.externalID()).
		UpdateDirectoryTypeRequestPayload(d.toUpdateFeaturesApiPayload()).
		Execute()

	return d.cr, err
}

func (d *DirectoryClient) DeleteDirectory(ctx context.Context) error {
	// without an externalID we can't connect to the API
	if d.externalID() == "" {
		return errors.New(errMisUse)
	}

	_, _, err := d.btpClient.AccountsServiceClient.DirectoryOperationsAPI.DeleteDirectory(ctx, d.externalID()).Execute()

	return err
}

func (d *DirectoryClient) NeedsCreation(ctx context.Context) (bool, error) {
	if d.externalID() == "" {
		return true, nil
	}
	var err error
	d.cachedApi, err = d.getDirectory(ctx)

	return d.cachedApi == nil, err
}

func (d *DirectoryClient) getDirectory(ctx context.Context) (*accountclient.DirectoryResponseObject, error) {
	extID := d.externalID()
	// without an externalID we can't connect to the API
	if extID == "" {
		return nil, errors.New(errMisUse)
	}

	directory, raw, err := d.btpClient.AccountsServiceClient.DirectoryOperationsAPI.GetDirectory(ctx, extID).Execute()
	if raw.StatusCode == 404 {
		// Unfortunately the API has no error type for 404 errors, so we can only extract that from raw status
		return nil, nil
	}
	if err != nil {
		return nil, specifyAPIError(err)
	}
	return directory, nil
}
func (d *DirectoryClient) NeedsUpdate(ctx context.Context) (bool, error) {
	if d.cachedApi == nil {
		var err error
		d.cachedApi, err = d.getDirectory(ctx)
		if err != nil {
			return false, err
		}
	}
	return !isSynced(d.cr, d.cachedApi), nil
}

func (d *DirectoryClient) CreateDirectory(ctx context.Context) (*v1alpha1.Directory, error) {
	directory, _, err := d.btpClient.AccountsServiceClient.DirectoryOperationsAPI.
		CreateDirectory(ctx).
		ParentGUID(d.cr.Spec.ForProvider.DirectoryGuid).
		CreateDirectoryRequestPayload(d.toCreateApiPayload()).
		Execute()

	if err != nil {
		return d.cr, specifyAPIError(err)
	}
	meta.SetExternalName(d.cr, directory.Guid)
	return d.cr, nil
}

func (d *DirectoryClient) SyncStatus(ctx context.Context) error {
	if d.cachedApi == nil {
		var err error
		d.cachedApi, err = d.getDirectory(ctx)
		if err != nil {
			return err
		}
	}

	d.cr.Status.AtProvider.Guid = &d.cachedApi.Guid
	d.cr.Status.AtProvider.EntityState = d.cachedApi.EntityState
	d.cr.Status.AtProvider.StateMessage = d.cachedApi.StateMessage
	d.cr.Status.AtProvider.Subdomain = d.cachedApi.Subdomain
	d.cr.Status.AtProvider.DirectoryFeatures = d.cachedApi.DirectoryFeatures

	return nil
}

func (d *DirectoryClient) IsAvailable() bool {
	if d.cr.Status.AtProvider.EntityState == nil {
		return false
	}
	return *d.cr.Status.AtProvider.EntityState == v1alpha1.DirectoryEntityStateOk
}

func (d *DirectoryClient) externalID() string {
	extName := meta.GetExternalName(d.cr)

	if _, err := uuid.Parse(extName); err != nil {
		return ""
	}
	return extName
}

func isSynced(cr *v1alpha1.Directory, api *accountclient.DirectoryResponseObject) bool {
	providedDirectoryFeatures := cr.Spec.ForProvider.DirectoryFeatures
	if providedDirectoryFeatures == nil {
		providedDirectoryFeatures = []string{"DEFAULT"}
	}
	return cr.Spec.ForProvider.Description == api.Description &&
		internal.Val(cr.Spec.ForProvider.DisplayName) == api.DisplayName &&
		reflect.DeepEqual(cr.Spec.ForProvider.Labels, internal.Val(api.Labels)) &&
		reflect.DeepEqual(providedDirectoryFeatures, api.DirectoryFeatures)
}

func (d *DirectoryClient) toUpdateApiPayload() accountclient.UpdateDirectoryRequestPayload {
	payload := accountclient.UpdateDirectoryRequestPayload{
		Description: &d.cr.Spec.ForProvider.Description,
		DisplayName: d.cr.Spec.ForProvider.DisplayName,
		Labels:      &d.cr.Spec.ForProvider.Labels,
	}
	return payload
}

func (d *DirectoryClient) toUpdateFeaturesApiPayload() accountclient.UpdateDirectoryTypeRequestPayload {
	payload := accountclient.UpdateDirectoryTypeRequestPayload{
		DirectoryFeatures: d.cr.Spec.ForProvider.DirectoryFeatures,
		// those are actually applied only in case "AUTHORIZATIONS" are set as features, but does not hurt to send them always
		DirectoryAdmins: d.cr.Spec.ForProvider.DirectoryAdmins,
		Subdomain:       d.cr.Spec.ForProvider.Subdomain,
	}
	return payload
}

func (d *DirectoryClient) toCreateApiPayload() accountclient.CreateDirectoryRequestPayload {
	var displayName string
	if d.cr.Spec.ForProvider.DisplayName != nil {
		displayName = *d.cr.Spec.ForProvider.DisplayName
	}
	payload := accountclient.CreateDirectoryRequestPayload{
		Description:       &d.cr.Spec.ForProvider.Description,
		DirectoryAdmins:   d.cr.Spec.ForProvider.DirectoryAdmins,
		DirectoryFeatures: d.cr.Spec.ForProvider.DirectoryFeatures,
		DisplayName:       displayName,
		Labels:            &d.cr.Spec.ForProvider.Labels,
		Subdomain:         d.cr.Spec.ForProvider.Subdomain,
	}
	return payload
}

var _ DirectoryClientI = &DirectoryClient{}

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
