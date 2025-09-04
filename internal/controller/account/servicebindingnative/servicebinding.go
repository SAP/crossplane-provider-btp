package servicebindingnative

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/clients/account/servicebindingnative"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotResource           = "managed resource is not a ServiceBinding custom resource"
	errCheckNeedsCreation    = "cannot check if the resource needs creation"
	errCheckNeedsUpdate      = "cannot check if the resource needs an update"
	errCreate                = "cannot create resoure"
	errDelete                = "cannot delete resoure"
	errUpdate                = "cannot update resoure"
	errUpdateStatus          = "cannot update status of resoure"
	errDeleteExpiredBindings = "cannot delete expired bindings"
	errDeleteRetiredBindings = "cannot delete retired bindings"
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	usage           resource.Tracker
	resourcetracker tracking.ReferenceResolverTracker
	newServiceFn    func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*apisv1alpha1.ServiceBinding)
	if !ok {
		return nil, errors.New(errNotResource)
	}

	smc, err := servicebindingnative.NewServiceManagerClientIFromResource(ctx, cr, c.kube)
	if err != nil {
		return nil, err
	}
	client := servicebindingnative.NewServiceBindingClient(smc, cr, c.kube)

	return &external{
		kube:       c.kube,
		client:     client,
		tracker:    c.resourcetracker,
		keyRotator: servicebindingnative.NewSBKeyRotator(smc, cr),
	}, nil
}

// Disconnect is a no-op for the external client to close its connection.
// Since we dont need this, we only have it to fullfil the interface.
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	// A 'client' used to connect to the external resource API. In practice this
	// would be something like an AWS SDK client.
	kube       client.Client
	client     servicebindingnative.ServiceBindingClientI
	tracker    tracking.ReferenceResolverTracker
	keyRotator servicebindingnative.KeyRotator
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*apisv1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotResource)
	}

	cr.SetConditions(c.softValidation(cr))
	c.tracker.SetConditions(ctx, cr)

	// Needs create?
	if needs, err := c.client.NeedsCreation(ctx); err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errCheckNeedsCreation)
	} else if needs {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	creds, err := c.client.SyncStatus(ctx)
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	// Needs Update?
	if needs, err := c.client.NeedsUpdate(ctx); err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errCheckNeedsUpdate)
	} else if needs {
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: false,
		}, nil
	}

	switch cr.Status.AtProvider.LastOperation.Type { //nolint:exhaustive
	case apisv1alpha1.ServiceBindingLastOperationTypeCreate:
		switch cr.Status.AtProvider.LastOperation.State {
		case apisv1alpha1.ServiceBindingLastOperationStateSucceeded:
			cr.Status.SetConditions(xpv1.Available())
		case apisv1alpha1.ServiceBindingLastOperationStatePending:
			cr.Status.SetConditions(xpv1.Creating())
		}
	case apisv1alpha1.ServiceBindingLastOperationTypeDelete:
		switch cr.Status.AtProvider.LastOperation.State {
		case apisv1alpha1.ServiceBindingLastOperationStateSucceeded:
			cr.Status.SetConditions(xpv1.Unavailable())
		case apisv1alpha1.ServiceBindingLastOperationStatePending:
			cr.Status.SetConditions(xpv1.Deleting())
		}
	default:
		cr.Status.SetConditions(xpv1.Unavailable())
	}

	if c.keyRotator.RetireBinding() {
		if err := c.kube.Status().Update(ctx, cr); err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errUpdateStatus)
		}
		return managed.ExternalObservation{ResourceUpToDate: false, ResourceExists: false}, nil
	}

	if c.keyRotator.HasExpiredBindings() {
		return managed.ExternalObservation{ResourceUpToDate: false, ResourceExists: true}, nil
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true,
		ConnectionDetails: creds,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	_, cd, err := c.client.CreateServiceBinding(ctx)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	if err := c.kube.Update(ctx, mg); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errUpdateStatus)
	}

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails(cd),
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*apisv1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotResource)
	}

	if _, err := c.client.UpdateServiceBinding(ctx); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	if err := c.keyRotator.DeleteExpiredBindings(ctx); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errDeleteExpiredBindings)
	}
	if err := c.kube.Status().Update(ctx, cr); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateStatus)
	}

	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*apisv1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotResource)
	}

	c.tracker.SetConditions(ctx, cr)
	if blocked := c.tracker.DeleteShouldBeBlocked(mg); blocked {
		return managed.ExternalDelete{}, errors.New(providerv1alpha1.ErrResourceInUse)
	}

	if err := c.keyRotator.DeleteRetiredBindings(ctx); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteRetiredBindings)
	}

	if err := c.client.DeleteServiceBinding(ctx); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	cr.SetConditions(xpv1.Deleting())
	return managed.ExternalDelete{}, nil
}

// softValidation adds conditions to the CR in order to guide the user with the usage of the Entitlements.
func (c *external) softValidation(cr *apisv1alpha1.ServiceBinding) xpv1.Condition {
	_ = cr

	var errs []string

	return apisv1alpha1.ValidationCondition(errs)
}
