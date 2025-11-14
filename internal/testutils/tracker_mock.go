package testutils

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

// ResourceTrackerMock is a mock implementation of tracking.ReferenceResolverTracker
// for use in unit tests across the codebase.
var _ tracking.ReferenceResolverTracker = &ResourceTrackerMock{}

type ResourceTrackerMock struct {
	// Track method fields
	TrackCalled     bool
	TrackedResource resource.Managed
	TrackErr        error

	// SetConditions method fields
	SetConditionsCalled bool

	// DeleteShouldBeBlocked method fields
	ShouldBlock bool
}

func (r *ResourceTrackerMock) Track(ctx context.Context, mg resource.Managed) error {
	r.TrackCalled = true
	r.TrackedResource = mg
	return r.TrackErr
}

func (r *ResourceTrackerMock) SetConditions(ctx context.Context, mg resource.Managed) {
	r.SetConditionsCalled = true
}

func (r *ResourceTrackerMock) ResolveSource(ctx context.Context, ru providerv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error) {
	return nil, nil
}

func (r *ResourceTrackerMock) ResolveTarget(ctx context.Context, ru providerv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error) {
	return nil, nil
}

func (r *ResourceTrackerMock) DeleteShouldBeBlocked(mg resource.Managed) bool {
	return r.ShouldBlock
}

// NewResourceTrackerMock creates a new ResourceTrackerMock with default values
func NewResourceTrackerMock() *ResourceTrackerMock {
	return &ResourceTrackerMock{
		TrackCalled:         false,
		SetConditionsCalled: false,
		ShouldBlock:         false,
	}
}

// NewResourceTrackerMockWithError creates a mock that returns an error on Track()
func NewResourceTrackerMockWithError(err error) *ResourceTrackerMock {
	return &ResourceTrackerMock{
		TrackErr: err,
	}
}

// NewResourceTrackerMockBlocking creates a mock that blocks deletion
func NewResourceTrackerMockBlocking() *ResourceTrackerMock {
	return &ResourceTrackerMock{
		ShouldBlock: true,
	}
}
