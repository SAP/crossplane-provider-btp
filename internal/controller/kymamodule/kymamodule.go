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
		return managed.ExternalObservation{}, err
	}

	res, err := c.client.ObserveModule(ctx, cr)

	if err != nil {
		return managed.ExternalObservation{}, err
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
	isValid, kymaCreds, err := getValidKubeconfig(secret)

	// If the kubeconfig is not valid we wait for the secret to be updated
	// If the kymaCreds are empty, we asume the secret is corrupted
	// If the parsing fails we return an error
	if !isValid {
		return nil
	}
	if len(kymaCreds) == 0 {
		return errors.New(errGetCredentialsSecret)
	}
	if err != nil {
		return err
	}

	// Generate a new KymaModuleClient with the kubeconfig from the secret
	client, err := kymamodule.NewKymaModuleClient(kymaCreds)
	if err != nil {
		return errors.Wrap(err, errFailedCreateClient)
	}

	// Set the new client to the external client
	c.client = client
	return nil
}

// getValidKubeconfig checks if the secret contains valid kubeconfig data and is not expired
func getValidKubeconfig(secret map[string][]byte) (bool, []byte, error) {

	// Check expiration if present
	expirationBytes := secret[v1alpha1.KymaEnvironmentBindingExpirationKey]
	expiration, err := time.Parse(kymaExpirationLayout, string(expirationBytes))
	if err != nil {
		// Parsing has failed
		return false, nil, errors.Wrap(err, errTimeParser)
	}
	if expiration.Before(time.Now()) {
		// Secret has expired
		return false, nil, nil
	}

	// Get credentials
	creds := secret[v1alpha1.KymaEnvironmentBindingKey]

	return true, creds, nil
}
