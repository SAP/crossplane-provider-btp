package rolecollectionassignment

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RoleAssignerMock struct {
	hasRole bool
	err     error

	CalledIdentifier *string
}

func (u *RoleAssignerMock) RevokeRole(ctx context.Context, origin, identifier, rolecollection string) error {
	u.CalledIdentifier = &identifier
	return u.err
}

func (u *RoleAssignerMock) AssignRole(ctx context.Context, origin, identifier, rolecollection string) error {
	u.CalledIdentifier = &identifier
	return u.err
}

func (u *RoleAssignerMock) HasRole(ctx context.Context, origin, identifier, roleCollection string) (bool, error) {
	u.CalledIdentifier = &identifier
	return u.hasRole, u.err
}

var _ RoleAssigner = &RoleAssignerMock{}

// ReferenceResolverTrackerMock is a mock implementation of ReferenceResolverTracker interface
type ReferenceResolverTrackerMock struct{}

func (r *ReferenceResolverTrackerMock) Track(ctx context.Context, mg resource.Managed) error {
	return nil
}

func (r *ReferenceResolverTrackerMock) SetConditions(ctx context.Context, mg resource.Managed) {
	// No-op for mock
}
func (r *ReferenceResolverTrackerMock) ResolveSource(
	ctx context.Context,
	ru v1alpha1.ResourceUsage,
) (*metav1.PartialObjectMetadata, error) {
	return &metav1.PartialObjectMetadata{}, nil
}
func (r *ReferenceResolverTrackerMock) ResolveTarget(
	ctx context.Context,
	ru v1alpha1.ResourceUsage,
) (*metav1.PartialObjectMetadata, error) {
	return &metav1.PartialObjectMetadata{}, nil
}
func (r *ReferenceResolverTrackerMock) DeleteShouldBeBlocked(mg resource.Managed) bool {
	return false
}
