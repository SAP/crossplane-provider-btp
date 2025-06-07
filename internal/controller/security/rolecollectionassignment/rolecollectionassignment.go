package rolecollectionassignment

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"

	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/btp"
	rolecollectiongroupassignment "github.com/sap/crossplane-provider-btp/internal/clients/security/rolecollectiongroupassignment"
	"github.com/sap/crossplane-provider-btp/internal/clients/security/rolecollectionuserassignment"
	"github.com/sap/crossplane-provider-btp/internal/tracking"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

const (
	errNotRoleCollectionAssignment = "managed resource is not a RoleCollectionAssignment custom resource"
	errTrackPCUsage                = "cannot track ProviderConfig usage"
	errTrackRCUsage                = "cannot track ResourceUsage"

	errGetSecret  = "api credential secret not found"
	errReadSecret = "api credential secret is malformed"

	errRetrieveRole = "cannot retrieve api data"
	errAssignRole   = "cannot assign role"
	errRevokeRole   = "cannot revoke role"

	errNotImplemented = "not implemented"
	errNewClient      = "cannot create new Service"
)

var (
	errInvalidSecret = errors.New("api credential secret invalid")
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
	usage              resource.Tracker
	newUserAssignerFn  func(binding *v1alpha1.XsuaaBinding) (RoleAssigner, error)
	newGroupAssignerFn func(binding *v1alpha1.XsuaaBinding) (RoleAssigner, error)
	resourcetracker    tracking.ReferenceResolverTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.RoleCollectionAssignment)
	if !ok {
		return nil, errors.New(errNotRoleCollectionAssignment)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
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

	return &external{client: svc}, nil
}

type external struct {
	client RoleAssigner
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.RoleCollectionAssignment)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotRoleCollectionAssignment)
	}

	hasRole, err := c.client.HasRole(ctx, cr.Spec.ForProvider.Origin, IdentifierName(cr), cr.Spec.ForProvider.RoleCollectionName)

	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errRetrieveRole)
	}
	if !hasRole {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
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

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	return managed.ExternalUpdate{}, errors.New(errNotImplemented)
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.RoleCollectionAssignment)
	if !ok {
		return errors.New(errNotRoleCollectionAssignment)
	}

	cr.Status.SetConditions(xpv1.Deleting())
	err := c.client.RevokeRole(ctx, cr.Spec.ForProvider.Origin, IdentifierName(cr), cr.Spec.ForProvider.RoleCollectionName)
	if err != nil {
		return errors.Wrap(err, errRevokeRole)
	}

	return nil
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
