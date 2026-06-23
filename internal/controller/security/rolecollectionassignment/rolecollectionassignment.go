package rolecollectionassignment

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/btp"
	rolecollectiongroupassignment "github.com/sap/crossplane-provider-btp/internal/clients/security/rolecollectiongroupassignment"
	"github.com/sap/crossplane-provider-btp/internal/clients/security/rolecollectionuserassignment"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

const (
	errNotRoleCollectionAssignment = "managed resource is not a RoleCollectionAssignment custom resource"
	errTrackPCUsage                = "cannot track ProviderConfig usage"
	errTrackRCUsage                = "cannot track ResourceUsage"

	errGetSecret = "api credential secret not found"

	errRetrieveRole = "cannot retrieve api data"
	errAssignRole   = "cannot assign role"
	errRevokeRole   = "cannot revoke role"

	errParseExternalName  = "cannot parse external-name"
	errUpdateExternalName = "cannot update external-name to compound key"
	errNewClient          = "cannot create new Service"
)

var (
	errInvalidSecret            = errors.New("api credential secret invalid")
	ErrExternalNameSpecMismatch = errors.New("external-name does not match spec")
)

var _ RoleAssigner = &rolecollectionuserassignment.XsusaaUserRoleAssigner{}

var configureUserAssignerFn = func(binding *v1alpha1.XsuaaBinding) (RoleAssigner, error) {
	if binding == nil {
		return nil, errInvalidSecret
	}
	return rolecollectionuserassignment.NewXsuaaUserRoleAssigner(btp.NewBackgroundContextWithDebugPrintHTTPClient(), binding.ClientId, binding.ClientSecret, binding.TokenURL, binding.ApiUrl), nil
}

var configureGroupAssignerFn = func(binding *v1alpha1.XsuaaBinding) (RoleAssigner, error) {
	if binding == nil {
		return nil, errInvalidSecret
	}
	return rolecollectiongroupassignment.NewXsuaaGroupRoleAssigner(btp.NewBackgroundContextWithDebugPrintHTTPClient(), binding.ClientId, binding.ClientSecret, binding.TokenURL, binding.ApiUrl), nil
}

type RoleAssigner interface {
	HasRole(ctx context.Context, origin, name, roleCollection string) (bool, error)
	AssignRole(ctx context.Context, origin, name, rolecollection string) error
	RevokeRole(ctx context.Context, origin, name, rolecollection string) error
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube               client.Client
	usage              providerconfig.LegacyTracker
	newUserAssignerFn  func(binding *v1alpha1.XsuaaBinding) (RoleAssigner, error)
	newGroupAssignerFn func(binding *v1alpha1.XsuaaBinding) (RoleAssigner, error)
	resourcetracker    tracking.ReferenceResolverTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.RoleCollectionAssignment)
	if !ok {
		return nil, errors.New(errNotRoleCollectionAssignment)
	}

	if err := c.usage.Track(ctx, mg.(resource.LegacyManaged)); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	if err := c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackRCUsage)
	}

	binding, err := v1alpha1.CreateBindingFromSource(&cr.Spec.XSUAACredentialsReference, ctx, c.kube)

	if err != nil {
		return nil, errors.Wrap(err, errGetSecret)
	}

	svc, err := c.newService(cr, binding)

	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{client: svc, kube: c.kube}, nil
}

type external struct {
	client RoleAssigner
	kube   client.Client
}

// Disconnect is a no-op for the external client to close its connection.
// Since we dont need this, we only have it to fullfil the interface.
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.RoleCollectionAssignment)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotRoleCollectionAssignment)
	}

	externalName := meta.GetExternalName(cr)

	// Empty annotation means a fresh resource (the default initializer is
	// suppressed via DefaultSetupWithoutDefaultInitializer). Trigger Create;
	// if the assignment already exists, AssignRole returns a conflict and we
	// stay in an error loop until the user sets the compound external-name.
	// ADR(external-name): Definition #1 — empty annotation expresses "create
	// new", not "adopt existing".
	if externalName == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Legacy sentinel: pre-ADR controller wrote `cr.Name` via the default
	// initializer. ADR(external-name): Observe step 1 backwards-compat branch.
	if externalName == cr.Name {
		return c.observeLegacy(ctx, cr)
	}

	origin, name, roleCollection, err := ParseExternalName(externalName)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errParseExternalName)
	}

	// All four spec fields are immutable via XValidation, so the only way the
	// parsed external-name can disagree with the spec is at import time when
	// the user typed inconsistent values. Reject loudly: refuse to take
	// ownership until the manifest is consistent with the resource being
	// adopted. The user must fix either the annotation or the spec — there is
	// nothing the controller can reconcile on its own.
	if mismatch := externalNameSpecMismatch(cr, origin, name, roleCollection); mismatch != "" {
		return managed.ExternalObservation{}, errors.Wrap(ErrExternalNameSpecMismatch, mismatch)
	}

	hasRole, err := c.client.HasRole(ctx, origin, name, roleCollection)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errRetrieveRole)
	}
	if !hasRole {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	cr.Status.SetConditions(xpv1.Available())
	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

// observeLegacy handles the pre-ADR `externalName == cr.Name` sentinel: query
// XSUAA using the spec values; on hit, rewrite the annotation to the compound
// key; on miss, signal Create.
func (c *external) observeLegacy(ctx context.Context, cr *v1alpha1.RoleCollectionAssignment) (managed.ExternalObservation, error) {
	hasRole, err := c.client.HasRole(ctx, cr.Spec.ForProvider.Origin, IdentifierName(cr), cr.Spec.ForProvider.RoleCollectionName)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errRetrieveRole)
	}
	if !hasRole {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	meta.SetExternalName(cr, BuildExternalName(cr))
	if err := c.kube.Update(ctx, cr); err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errUpdateExternalName)
	}

	cr.Status.SetConditions(xpv1.Available())
	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.RoleCollectionAssignment)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotRoleCollectionAssignment)
	}

	cr.Status.SetConditions(xpv1.Creating())

	err := c.client.AssignRole(ctx, cr.Spec.ForProvider.Origin, IdentifierName(cr), cr.Spec.ForProvider.RoleCollectionName)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errAssignRole)
	}

	meta.SetExternalName(cr, BuildExternalName(cr))

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	// All spec fields are immutable via XValidation, so Update has no work to do.
	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.RoleCollectionAssignment)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotRoleCollectionAssignment)
	}

	cr.Status.SetConditions(xpv1.Deleting())

	origin, name, roleCollection, err := ParseExternalName(meta.GetExternalName(cr))
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errParseExternalName)
	}

	if err := c.client.RevokeRole(ctx, origin, name, roleCollection); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errRevokeRole)
	}

	return managed.ExternalDelete{}, nil
}

// newService chooses one of the serviceCreation functions based on the type of the RoleCollectionAssignment
func (c *connector) newService(cr *v1alpha1.RoleCollectionAssignment, binding *v1alpha1.XsuaaBinding) (RoleAssigner, error) {
	if isUserAssignment(cr) {
		return c.newUserAssignerFn(binding)
	}
	return c.newGroupAssignerFn(binding)
}

// isUserAssignment checks if the rolecollection assignment is for a user or a group
func isUserAssignment(cr *v1alpha1.RoleCollectionAssignment) bool {
	// consistency of set username or group is enforced on schema level
	return cr.Spec.ForProvider.UserName != ""
}

// IdentifierName returns the identifier for the entity to be assigned to the rolecollection (username or groupname)
func IdentifierName(cr *v1alpha1.RoleCollectionAssignment) string {
	if cr.Spec.ForProvider.UserName != "" {
		return cr.Spec.ForProvider.UserName
	}
	return cr.Spec.ForProvider.GroupName
}
