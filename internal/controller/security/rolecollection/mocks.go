package rolecollection

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	v1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RoleMaintainerMock is a mock implementation of RoleCollectionMaintainer interface
// returns stubed values and records called identifier to most methods
type RoleMaintainerMock struct {
	generateObservation securityv1alpha1.RoleCollectionObservation
	needsCreation       bool
	needsUpdate         bool
	err                 error
	// for verification
	CalledIdentifier string
}

var _ RoleCollectionMaintainer = &RoleMaintainerMock{}

func (r *RoleMaintainerMock) GenerateObservation(ctx context.Context, roleCollectionName string) (securityv1alpha1.RoleCollectionObservation, error) {
	r.CalledIdentifier = roleCollectionName
	return r.generateObservation, r.err
}

func (r *RoleMaintainerMock) NeedsCreation(collection securityv1alpha1.RoleCollectionObservation) bool {
	return r.needsCreation
}

func (r *RoleMaintainerMock) NeedsUpdate(params securityv1alpha1.RoleCollectionParameters, observation securityv1alpha1.RoleCollectionObservation) bool {
	return r.needsUpdate
}

func (r *RoleMaintainerMock) Create(ctx context.Context, params securityv1alpha1.RoleCollectionParameters) (string, error) {
	return r.CalledIdentifier, r.err
}

func (r *RoleMaintainerMock) Update(ctx context.Context, roleCollectionName string, params securityv1alpha1.RoleCollectionParameters, obs securityv1alpha1.RoleCollectionObservation) error {
	r.CalledIdentifier = roleCollectionName
	return r.err
}

func (r *RoleMaintainerMock) Delete(ctx context.Context, roleCollectionName string) error {
	r.CalledIdentifier = roleCollectionName
	return r.err
}

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
