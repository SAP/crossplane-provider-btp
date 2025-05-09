package test

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
)

// NoOpReferenceResolverTracker with blocked delete for testing purposes
type NoOpReferenceResolverTrackerBlocked struct {
}

func (n NoOpReferenceResolverTrackerBlocked) Track(ctx context.Context, mg resource.Managed) error {
	return nil
}

func (n NoOpReferenceResolverTrackerBlocked) SetConditions(ctx context.Context, mg resource.Managed) {
	// No-op
}

func (n NoOpReferenceResolverTrackerBlocked) ResolveSource(ctx context.Context, ru providerv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error) {
	return nil, nil
}

func (n NoOpReferenceResolverTrackerBlocked) ResolveTarget(ctx context.Context, ru providerv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error) {
	return nil, nil
}

func (n NoOpReferenceResolverTrackerBlocked) DeleteShouldBeBlocked(mg resource.Managed) bool {
	return true
}
