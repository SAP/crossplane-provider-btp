package test

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
)

// NoOpReferenceResolverTracker For testing purposes
type NoOpReferenceResolverTracker struct {
	// IsResourceBlocked indicates whether the resource should be blocked for deletion or not
	IsResourceBlocked bool
}

func (n NoOpReferenceResolverTracker) Track(ctx context.Context, mg resource.Managed) error {
	return nil
}

func (n NoOpReferenceResolverTracker) SetConditions(ctx context.Context, mg resource.Managed) {
	// No-op
}

func (n NoOpReferenceResolverTracker) ResolveSource(ctx context.Context, ru providerv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error) {
	return nil, nil
}

func (n NoOpReferenceResolverTracker) ResolveTarget(ctx context.Context, ru providerv1alpha1.ResourceUsage) (*metav1.PartialObjectMetadata, error) {
	return nil, nil
}

func (n NoOpReferenceResolverTracker) DeleteShouldBeBlocked(mg resource.Managed) bool {
	return n.IsResourceBlocked
}
