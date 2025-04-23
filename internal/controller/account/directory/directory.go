package directory

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/clients/directory"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"

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

func (c *external) Disconnect(ctx context.Context) error {
	// No-op
	return nil
}
func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Directory)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotDirectory)
	}

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

	cr.SetConditions(xpv1.Deleting())

	return managed.ExternalDelete{}, c.handler(cr).DeleteDirectory(ctx)
}

func (c *external) handler(cr *v1alpha1.Directory) directory.DirectoryClientI {
	return c.newDirHandlerFn(c.btpClient, cr)
}
