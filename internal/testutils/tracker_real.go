package testutils

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

// NewRealResourceTracker returns a real DefaultReferenceResolverTracker backed by a
// controller-runtime fake client. When inUse is true the client is seeded with a single
// ResourceUsage whose source-UID label equals mg's UID, so tracker.SetConditions(ctx, mg)
// stamps an InUse condition; when false, no usages exist and it stamps NotInUse.
//
// Set mg's UID before calling.
func NewRealResourceTracker(t *testing.T, mg resource.Managed, inUse bool) tracking.ReferenceResolverTracker {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatalf("NewRealResourceTracker: add scheme: %v", err)
	}
	builder := fake.NewClientBuilder().WithScheme(scheme)
	if inUse {
		builder = builder.WithObjects(&providerv1alpha1.ResourceUsage{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "ru-" + string(mg.GetUID()),
				Labels: map[string]string{providerv1alpha1.LabelKeySourceUid: string(mg.GetUID())},
			},
		})
	}
	return tracking.NewDefaultReferenceResolverTracker(builder.Build())
}
