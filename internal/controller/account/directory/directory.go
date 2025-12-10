package directory

import (
	"context"
	"fmt"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/clients/directory"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

const (
	errNotDirectory = "managed resource is not a Directory custom resource"
)

var newDirHandlerFn = func(client *btp.Client, cr *v1alpha1.Directory) directory.DirectoryClientI {
	return directory.NewDirectoryClient(client, cr)
}

type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)

	newDirHandlerFn func(client *btp.Client, cr *v1alpha1.Directory) directory.DirectoryClientI

	resourcetracker tracking.ReferenceResolverTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	_, ok := mg.(*v1alpha1.Directory)
	if !ok {
		return nil, errors.New(errNotDirectory)
	}

	btpClient, err := providerconfig.CreateClient(ctx, mg, c.kube, c.usage, c.newServiceFn, c.resourcetracker)
	if err != nil {
		return nil, err
	}

	return &external{
		kube:            c.kube,
		btpClient:       btpClient,
		newDirHandlerFn: c.newDirHandlerFn,
		tracker:         c.resourcetracker,
	}, nil
}

type external struct {
	btpClient       *btp.Client
	newDirHandlerFn func(client2 *btp.Client, cr *v1alpha1.Directory) directory.DirectoryClientI

	kube    client.Client
	tracker tracking.ReferenceResolverTracker
}

// Disconnect is a no-op for the external client to close its connection.
// Since we dont need this, we only have it to fullfil the interface.
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Directory)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotDirectory)
	}

	// ADR Step 1: Check if external-name is empty
	if meta.GetExternalName(cr) == "" {
		// Backwards compatibility not needed, therefore return ResourceExists: false
		return managed.ExternalObservation{ResourceExists: false}, nil

	}

	// ADR Step 2: External-name is set, check its format (must be valid GUID)
	if !isValidUUID(meta.GetExternalName(cr)) {
		return managed.ExternalObservation{}, errors.New(fmt.Sprintf("external-name '%s' is not a valid GUID format", meta.GetExternalName(cr)))
	}

	// ADR Step 3: Build the Get API Request from the external-name (using GUID directly)
	directoryHandler := c.handler(cr)

	needsCreation, createErr := directoryHandler.NeedsCreation(ctx)
	if createErr != nil {
		return managed.ExternalObservation{}, createErr
	}

	if needsCreation {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	syncErr := directoryHandler.SyncStatus(ctx)

	if syncErr != nil {
		return managed.ExternalObservation{}, syncErr
	}

	// in case of updating the directoryFeatures instance gets unavailable for a while
	if !directoryHandler.IsAvailable() {
		cr.SetConditions(xpv1.Unavailable())

		return managed.ExternalObservation{ResourceExists: true,
			ResourceUpToDate:  true,
			ConnectionDetails: managed.ConnectionDetails{}}, nil
	}

	cr.SetConditions(xpv1.Available())

	needsUpdate, uErr := directoryHandler.NeedsUpdate(ctx)
	if (uErr) != nil {
		return managed.ExternalObservation{}, uErr
	}

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  !needsUpdate,
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Directory)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotDirectory)
	}

	directoryHandler := c.handler(cr)

	cr.SetConditions(xpv1.Creating())
	_, clientErr := directoryHandler.CreateDirectory(ctx)
	if clientErr != nil {
		return managed.ExternalCreation{}, clientErr
	}

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Directory)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotDirectory)
	}

	_, err := c.handler(cr).UpdateDirectory(ctx)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Directory)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotDirectory)
	}

	// ADR: Check if resource is already in deletion state
	if cr.Status.AtProvider.EntityState != nil && *cr.Status.AtProvider.EntityState == "DELETING" {
		return managed.ExternalDelete{}, nil
	}

	cr.SetConditions(xpv1.Deleting())

	err := c.handler(cr).DeleteDirectory(ctx)
	// ADR: 404 not found means already deleted - not considered as error case
	if err != nil && isNotFoundError(err) {
		ctrl.Log.Info("associated BTP directory not found, continue deletion")
		return managed.ExternalDelete{}, nil
	}

	return managed.ExternalDelete{}, err
}

func (c *external) handler(cr *v1alpha1.Directory) directory.DirectoryClientI {
	return c.newDirHandlerFn(c.btpClient, cr)
}

// isValidUUID checks if a string is a valid UUID format
func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// isNotFoundError checks if an error is a 404 not found error
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "directory not found"
}
