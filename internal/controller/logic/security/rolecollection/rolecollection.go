package rolecollection

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	base "github.com/sap/crossplane-provider-btp/apis/base/security/v1alpha1"
	legacyv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	service "github.com/sap/crossplane-provider-btp/internal/clients/security/rolecollection"
	"github.com/sap/crossplane-provider-btp/internal/controller/logic"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errGetSecret            = "api credential secret not found"
	errNewClient            = "cannot create new Service"
	errGetRolecollection    = "cannot get rolecollection"
	errCreateRolecollection = "cannot create rolecollection"
	errUpdateRolecollection = "cannot update rolecollection"
	errDeleteRolecollection = "cannot delete rolecollection"
)

var errInvalidSecret = errors.New("api credential secret invalid")

// Options is the per-Setup configuration for the RoleCollection controller.
type Options = logic.Options

// RoleCollectionMaintainer defines the contract for XSUAA role collection operations.
type RoleCollectionMaintainer interface {
	GenerateObservation(ctx context.Context, roleCollectionName string) (base.BaseRoleCollectionObservation, error)
	NeedsCreation(observation base.BaseRoleCollectionObservation) bool
	NeedsUpdate(params base.BaseRoleCollectionParameters, observation base.BaseRoleCollectionObservation) bool
	Create(ctx context.Context, params base.BaseRoleCollectionParameters) (string, error)
	Update(ctx context.Context, roleCollectionName string, params base.BaseRoleCollectionParameters, obs base.BaseRoleCollectionObservation) error
	Delete(ctx context.Context, roleCollectionName string) error
}

var configureRoleCollectionMaintainerFn = func(ctx context.Context, binding *legacyv1alpha1.XsuaaBinding) (RoleCollectionMaintainer, error) {
	if binding == nil {
		return nil, errInvalidSecret
	}
	return service.NewXsuaaRoleCollectionMaintainer(ctx, binding.ClientId, binding.ClientSecret, binding.TokenURL, binding.ApiUrl), nil
}

// Client holds the kube client for XSUAA credential resolution and the tracker for
// SetConditions / DeleteShouldBeBlocked.
type Client struct {
	kube    client.Client
	tracker tracking.ReferenceResolverTracker
}

// Setup wires up the RoleCollection controller via the shared logic.Setup helper.
func Setup(mgr ctrl.Manager, o Options, obj client.Object, kind string, gvk schema.GroupVersionKind, mk logic.MakeExternal) error {
	return logic.Setup(mgr, o, obj, kind, gvk, mk)
}

// Connect returns a Client carrying the kube client and a fresh tracker. RoleCollection
// reads XSUAA credentials directly from a secret resolved via the API credential
// reference, so no btp.Client construction is needed here.
func Connect(_ context.Context, _ resource.Managed, kube client.Client) (Client, error) {
	return Client{
		kube:    kube,
		tracker: tracking.NewDefaultReferenceResolverTracker(kube),
	}, nil
}

// MigrateExternalName is a no-op for RoleCollection (uses name as identifier, not a GUID).
func MigrateExternalName(_ Client, _ context.Context, _ resource.Managed, _ *base.BaseRoleCollection, _ client.Client) error {
	return nil
}

// Observe checks the external state of a role collection.
func Observe(c Client, ctx context.Context, mg resource.Managed, cr *base.BaseRoleCollection) (managed.ExternalObservation, error) {
	if c.tracker != nil {
		c.tracker.SetConditions(ctx, mg)
	}

	externalName := meta.GetExternalName(cr)
	if externalName == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	maintainer, err := createMaintainer(c, ctx, cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	obs, err := maintainer.GenerateObservation(ctx, externalName)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetRolecollection)
	}

	cr.Status.AtProvider = obs

	if maintainer.NeedsCreation(obs) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	cr.Status.SetConditions(xpv1.Available())

	if maintainer.NeedsUpdate(cr.Spec.ForProvider, obs) {
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

// Create provisions a new role collection.
func Create(c Client, ctx context.Context, _ resource.Managed, cr *base.BaseRoleCollection) (managed.ExternalCreation, error) {
	cr.Status.SetConditions(xpv1.Creating())

	maintainer, err := createMaintainer(c, ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	extName, err := maintainer.Create(ctx, cr.Spec.ForProvider)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateRolecollection)
	}

	meta.SetExternalName(cr, extName)

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

// Update modifies an existing role collection.
func Update(c Client, ctx context.Context, _ resource.Managed, cr *base.BaseRoleCollection) (managed.ExternalUpdate, error) {
	maintainer, err := createMaintainer(c, ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	if err := maintainer.Update(ctx, meta.GetExternalName(cr), cr.Spec.ForProvider, cr.Status.AtProvider); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateRolecollection)
	}

	return managed.ExternalUpdate{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

// Delete removes a role collection.
func Delete(c Client, ctx context.Context, mg resource.Managed, cr *base.BaseRoleCollection) (managed.ExternalDelete, error) {
	if c.tracker != nil {
		c.tracker.SetConditions(ctx, mg)
		if c.tracker.DeleteShouldBeBlocked(mg) {
			return managed.ExternalDelete{}, logic.DeleteBlockedError()
		}
	}

	cr.Status.SetConditions(xpv1.Deleting())

	maintainer, err := createMaintainer(c, ctx, cr)
	if err != nil {
		return managed.ExternalDelete{}, err
	}

	if err := maintainer.Delete(ctx, meta.GetExternalName(cr)); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteRolecollection)
	}

	return managed.ExternalDelete{}, nil
}

func createMaintainer(c Client, ctx context.Context, cr *base.BaseRoleCollection) (RoleCollectionMaintainer, error) {
	legacyRef := toLegacyCredentialsRef(&cr.Spec.XSUAACredentialsReference)
	binding, err := legacyv1alpha1.CreateBindingFromSource(legacyRef, ctx, c.kube)
	if err != nil {
		return nil, errors.Wrap(err, errGetSecret)
	}

	maintainer, err := configureRoleCollectionMaintainerFn(ctx, binding)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return maintainer, nil
}

func toLegacyCredentialsRef(ref *base.XSUAACredentialsReference) *legacyv1alpha1.XSUAACredentialsReference {
	return &legacyv1alpha1.XSUAACredentialsReference{
		APICredentials: legacyv1alpha1.APICredentials{
			Source:                    ref.APICredentials.Source,
			CommonCredentialSelectors: ref.APICredentials.CommonCredentialSelectors,
		},
		SubaccountApiCredentialSelector:        ref.SubaccountApiCredentialSelector,
		SubaccountApiCredentialRef:             ref.SubaccountApiCredentialRef,
		SubaccountApiCredentialSecret:          ref.SubaccountApiCredentialSecret,
		SubaccountApiCredentialSecretNamespace: ref.SubaccountApiCredentialSecretNamespace,
	}
}
