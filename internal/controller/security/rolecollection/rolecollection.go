package rolecollection

import (
	"context"

	"github.com/sap/crossplane-provider-btp/btp"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	service "github.com/sap/crossplane-provider-btp/internal/clients/security/rolecollection"
)

const (
	errNotRoleCollection = "managed resource is not a RoleCollection custom resource"
	errTrackPCUsage      = "cannot track ProviderConfig usage"

	errGetSecret = "api credential secret not found"

	errNewClient = "cannot create new Service"

	errGetRolecollection     = "cannot get rolecollection"
	errCreateRolecollection  = "cannot create rolecollection"
	errUpdateRolecollection  = "cannot update rolecollection"
	errDeleteeRolecollection = "cannot delete rolecollection"
)

var (
	errInvalidSecret = errors.New("api credential secret invalid")
)

type RoleCollectionMaintainer interface {
	GenerateObservation(ctx context.Context, roleCollectionName string) (v1alpha1.RoleCollectionObservation, error)

	NeedsCreation(collection v1alpha1.RoleCollectionObservation) bool
	NeedsUpdate(params v1alpha1.RoleCollectionParameters, observation v1alpha1.RoleCollectionObservation) bool

	Create(ctx context.Context, params v1alpha1.RoleCollectionParameters) (string, error)
	Update(ctx context.Context, roleCollectionName string, params v1alpha1.RoleCollectionParameters, obs v1alpha1.RoleCollectionObservation) error
	Delete(ctx context.Context, roleCollectionName string) error
}

var configureRoleCollectionMaintainerFn = func(secretData []byte) (RoleCollectionMaintainer, error) {
	binding, err := v1alpha1.ReadXsuaaCredentials(secretData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read xsuaa credentials.")
	}

	return service.NewXsuaaRoleCollectionMaintainer(btp.NewBackgroundContextWithDebugPrintHTTPClient(), binding.ClientId, binding.ClientSecret, binding.TokenURL, binding.ApiUrl), nil
}

type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func(creds []byte) (RoleCollectionMaintainer, error)
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.RoleCollection)
	if !ok {
		return nil, errors.New(errNotRoleCollection)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	secretBytes, err := resource.CommonCredentialExtractor(
		ctx,
		cr.Spec.APICredentials.Source,
		c.kube,
		cr.Spec.APICredentials.CommonCredentialSelectors,
	)

	if err != nil {
		return nil, errors.Wrap(err, errGetSecret)
	}
	if secretBytes == nil {
		return nil, errInvalidSecret
	}

	svc, err := c.newServiceFn(secretBytes)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{client: svc}, nil
}

type external struct {
	client RoleCollectionMaintainer
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.RoleCollection)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotRoleCollection)
	}

	obs, err := c.client.GenerateObservation(ctx, meta.GetExternalName(cr))
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetRolecollection)
	}

	setObservation(cr, obs)

	needsCreation := c.client.NeedsCreation(getObservation(cr))
	if needsCreation {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	cr.Status.SetConditions(xpv1.Available())

	needsUpdate := c.client.NeedsUpdate(getParams(cr), getObservation(cr))
	if needsUpdate {
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: false,
		}, nil
	}

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.RoleCollection)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotRoleCollection)
	}

	cr.Status.SetConditions(xpv1.Creating())

	extName, err := c.client.Create(ctx, cr.Spec.ForProvider)

	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateRolecollection)
	}

	meta.SetExternalName(cr, extName)

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.RoleCollection)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotRoleCollection)
	}

	if err := c.client.Update(ctx, meta.GetExternalName(cr), cr.Spec.ForProvider, cr.Status.AtProvider); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateRolecollection)
	}

	return managed.ExternalUpdate{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	// No-op
	return nil
}
func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.RoleCollection)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotRoleCollection)
	}

	cr.Status.SetConditions(xpv1.Deleting())

	if err := c.client.Delete(ctx, meta.GetExternalName(cr)); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteeRolecollection)
	}

	return managed.ExternalDelete{}, nil
}

// setObservation sets the observation within the CR
func setObservation(cr *v1alpha1.RoleCollection, obs v1alpha1.RoleCollectionObservation) {
	cr.Status.AtProvider = obs
}

// getObservation extracts the observation from the CR
func getObservation(cr *v1alpha1.RoleCollection) v1alpha1.RoleCollectionObservation {
	return cr.Status.AtProvider
}

// getParams extracts the parameters from the CR
func getParams(cr *v1alpha1.RoleCollection) v1alpha1.RoleCollectionParameters {
	return cr.Spec.ForProvider
}
