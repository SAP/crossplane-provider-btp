package kymamodule

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymamodule"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

var (
	errNotKymaModule   = "managed resource is not a KymaModule custom resource"
	errTrackPCUsage    = "cannot track ProviderConfig usage"
	errTrackRUsage     = "cannot track ResourceUsage"
	errSetupClient     = "cannot setup KymaModule client"
	errObserveResource = "cannot observe KymaModule"
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	usage           resource.Tracker
	resourcetracker tracking.ReferenceResolverTracker

	newServiceFn func(kymaEnvironmentKubeconfig []byte) (kymamodule.Client, error)
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	client        kymamodule.Client
	tracker       tracking.ReferenceResolverTracker
	kube          client.Client
	secretfetcher SecretFetcherInterface

	newServiceFn func(kymaEnvironmentKubeconfig []byte) (kymamodule.Client, error)
}

// This methods connects to the Kyma cluster using the kubeconfig from the KymaEnvironmentBinding
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.KymaModule)
	if !ok {
		return nil, errors.New(errNotKymaModule)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	if err := c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackRUsage)
	}

	secretfetcher := &SecretFetcher{
		kube: c.kube,
	}
	creds, err := secretfetcher.Fetch(ctx, cr)

	if err != nil {
		return nil, errors.Wrap(err, errSetupClient)
	}

	svc, err := c.newServiceFn(creds)

	return &external{
			client:        svc,
			tracker:       c.resourcetracker,
			kube:          c.kube,
			secretfetcher: secretfetcher,
			newServiceFn:  c.newServiceFn,
		},
		err
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.KymaModule)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotKymaModule)
	}

	// Check if the secret is valid and fetch the current kubeconfig
	creds, err := c.secretfetcher.Fetch(ctx, cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserveResource)
	}

	if creds == nil {
		return managed.ExternalObservation{}, errors.New(errCredentialsCorrupted)
	}

	// Renews the client if creds are provided
	client, err := c.newServiceFn(creds)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserveResource)
	}

	if client != nil {
		c.client = client
	}

	res, err := c.client.ObserveModule(ctx, cr)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserveResource)
	}

	if res == nil {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true,
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.KymaModule)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotKymaModule)
	}

	cr.Status.SetConditions(xpv1.Creating())

	err := c.client.CreateModule(ctx, cr.Spec.ForProvider.Name, *cr.Spec.ForProvider.Channel, *cr.Spec.ForProvider.CustomResourcePolicy)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	meta.SetExternalName(cr, cr.Spec.ForProvider.Name)

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {

	return managed.ExternalUpdate{}, errors.New("Update is not implemented - should not be called, only create")
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.KymaModule)
	if !ok {
		return errors.New(errNotKymaModule)
	}

	if cr.Status.AtProvider.State == v1alpha1.ModuleStateDeleting {
		return nil
	}

	err := c.client.DeleteModule(ctx, cr.Spec.ForProvider.Name)

	return err
}
