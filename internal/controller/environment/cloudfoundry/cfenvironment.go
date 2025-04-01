package cloudfoundry

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	env "github.com/sap/crossplane-provider-btp/internal/clients/cfenvironment"
	"github.com/sap/crossplane-provider-btp/internal/tracking"

	"github.com/sap/crossplane-provider-btp/btp"
)

const (
	errNotEnvironment          = "managed resource is not a CloudFoundryEnvironment custom resource"
	errExtractSecretKey        = "no Cloud Management Secret Found"
	errGetCredentialsSecret    = "could not get secret of local cloud management"
	errSecretDataInvalid       = "secret spec.Data.__raw is invalid"
	errUpdateNotSupported      = "update not supported"
	errTrackRUsage             = "cannot track ResourceUsage"
	errTrackPCUsage            = "cannot track ProviderConfig usage"
	errCreateConnectionDetails = "Cannot create connection details"

	errGetPC    = "cannot get ProviderConfig"
	errGetCreds = "cannot get credentials"
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	usage           resource.Tracker
	resourcetracker tracking.ReferenceResolverTracker

	newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.CloudFoundryEnvironment)
	if !ok {
		return nil, errors.New(errNotEnvironment)
	}

	pc := &providerv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: mg.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	if err := c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackRUsage)
	}

	if cr.Spec.CloudManagementSecret == "" || cr.Spec.CloudManagementSecretNamespace == "" {
		return nil, errors.New(errExtractSecretKey)
	}
	secret := &corev1.Secret{}
	if err := c.kube.Get(
		ctx, types.NamespacedName{
			Namespace: cr.Spec.CloudManagementSecretNamespace,
			Name:      cr.Spec.CloudManagementSecret,
		}, secret,
	); err != nil {
		return nil, errors.Wrap(err, errGetCredentialsSecret)
	}

	cd := pc.Spec.ServiceAccountSecret
	ServiceAccountSecretData, err := resource.CommonCredentialExtractor(
		ctx,
		cd.Source,
		c.kube,
		cd.CommonCredentialSelectors,
	)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	cisBinding := secret.Data[providerv1alpha1.RawBindingKey]
	if cisBinding == nil {
		return nil, errors.New(errGetCredentialsSecret)
	}
	svc, err := c.newServiceFn(cisBinding, ServiceAccountSecretData)

	return &external{client: env.NewCloudFoundryOrganization(*svc)}, err
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	client env.Client
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.CloudFoundryEnvironment)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotEnvironment)
	}

	instance, managers, err := c.client.DescribeInstance(ctx, *cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	cr.Status.AtProvider = env.GenerateObservation(instance, managers)

	if cr.Status.AtProvider.State != nil && *cr.Status.AtProvider.State == v1alpha1.InstanceStateOk {
		externalName := env.ExternalName(instance)
		if externalName != nil {
			meta.SetExternalName(cr, *externalName)
		}
		cr.Status.SetConditions(xpv1.Available())
	} else {
		cr.Status.SetConditions(xpv1.Unavailable())
	}

	if needsCreation := c.needsCreation(cr); needsCreation {
		return managed.ExternalObservation{
			ResourceExists: !needsCreation,
		}, nil
	}

	details, err := env.GetConnectionDetails(instance)
	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: details,
	}, errors.Wrap(err, errCreateConnectionDetails)
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.CloudFoundryEnvironment)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotEnvironment)
	}

	err := c.client.CreateInstance(ctx, *cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	_, ok := mg.(*v1alpha1.CloudFoundryEnvironment)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotEnvironment)
	}

	// Update is not supported
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.CloudFoundryEnvironment)
	if !ok {
		return errors.New(errNotEnvironment)
	}
	cr.SetConditions(xpv1.Deleting())

	return c.client.DeleteInstance(ctx, *cr)
}

func (c *external) needsCreation(cr *v1alpha1.CloudFoundryEnvironment) bool {
	return cr.Status.AtProvider.State == nil
}
