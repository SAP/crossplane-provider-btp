package kymamodule

import (
	"context"
	"time"

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

var (
	errNotKymaModule        = "managed resource is not a KymaModule custom resource"
	errTrackPCUsage         = "cannot track ProviderConfig usage"
	errTrackRUsage          = "cannot track ResourceUsage"
	errObserveResource      = "cannot observe KymaModule"
	errCredentialsCorrupted = "secret credentials data not in the expected format"
	errTimeParser           = "Failed to parse expiration time"
	errFailedCreateClient   = "failed to create KymaModule client"
	kymaExpirationLayout    = "2006-01-02 15:04:05.999999999 -0700 MST"
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

	// Check if the secret is valid and fetch the current kubeconfig
	err := c.fetchRotatingSecret(cr, ctx)

	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserveResource)
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

	err := c.client.DeleteModule(ctx, cr.Spec.ForProvider.Name)

	return err
}

func (c *external) fetchRotatingSecret(cr *v1alpha1.KymaModule, ctx context.Context) error {
	secretName := cr.Spec.KymaEnvironmentBindingSecret
	namespace := cr.Spec.KymaEnvironmentBindingSecretNamespace

	secret, errGet := internal.LoadSecret(ctx, c.kube, secretName, namespace)
	if errGet != nil {
		return errGet
	}

	// Check if the secret contains valid kubeconfig data that is not expired
	kymaCreds, err := getKubeconfig(secret)

	if err != nil {
		return err
	}

	// Renews the client if kymaCreds are provided
	if kymaCreds != nil {
		client, err := kymamodule.NewKymaModuleClient(kymaCreds)
		if err != nil {
			return errors.Wrap(err, errFailedCreateClient)
		}

		c.client = client
	}

	return nil
}

// getKubeconfig checks if the secret contains valid kubeconfig data and is not expired
func getKubeconfig(secret map[string][]byte) ([]byte, error) {

	expirationBytes := secret[v1alpha1.KymaEnvironmentBindingExpirationKey]
	if len(expirationBytes) == 0 {
		// No expiration time found
		return nil, errors.New(errCredentialsCorrupted)
	}

	expiration, err := time.Parse(kymaExpirationLayout, string(expirationBytes))
	if err != nil {
		// Parsing has failed
		return nil, errors.New(errTimeParser)
	}
	if expiration.Before(time.Now()) {
		// Secret has expired
		return nil, nil
	}

	creds := secret[v1alpha1.KymaEnvironmentBindingKey]
	if len(creds) == 0 {
		// No kubeconfig data found
		return nil, errors.New(errCredentialsCorrupted)
	}

	return creds, nil
}
