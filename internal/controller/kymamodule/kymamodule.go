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
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymamodule"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotKymaModule        = "managed resource is not a KymaModule custom resource"
	errTrackPCUsage         = "cannot track ProviderConfig usage"
	errGetPC                = "cannot get ProviderConfig"
	errGetCreds             = "cannot get credentials"
	errExtractSecretKey     = "no KymaEnvironmentBinding secret found"
	errGetCredentialsSecret = "could not get kubeconfig from KymaEnvironmentBinding secret"
	errTrackRUsage          = "cannot track ResourceUsage"
	errObserveResource      = "cannot observe KymaModule"
	errCredentialsCorrupted = "secret credentials data not in the expected format"
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	usage           resource.Tracker
	resourcetracker tracking.ReferenceResolverTracker

	newServiceFn func(kymaEnvironmentKubeconfig []byte) (*kymamodule.KymaModuleClient, error)
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	client  kymamodule.Client
	tracker tracking.ReferenceResolverTracker
	kube    client.Client
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

	secretName := cr.Spec.KymaEnvironmentBindingSecret
	namespace := cr.Spec.KymaEnvironmentBindingSecretNamespace

	secret, errGet := internal.LoadSecret(ctx, c.kube, secretName, namespace)
	if errGet != nil {
		return nil, errGet
	}

	kymaCreds := secret[v1alpha1.KymaEnvironmentBindingKey]

	svc, err := c.newServiceFn(kymaCreds)

	return &external{
			client:  svc,
			tracker: c.resourcetracker,
			kube:    c.kube,
		},
		err
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.KymaModule)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotKymaModule)
	}

	res, err := c.client.Observe(ctx, meta.GetExternalName(cr))

	// Update the status of the resource
	cr.Status.AtProvider = *res

	if err != nil {
		return managed.ExternalObservation{}, err
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

	err := c.client.Create(ctx, cr.Spec.ForProvider.Name, *cr.Spec.ForProvider.Channel, *cr.Spec.ForProvider.CustomResourcePolicy)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	// Enables importing existing modules by setting the external name
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

	err := c.client.Delete(ctx, cr.Spec.ForProvider.Name)

	return err
}
