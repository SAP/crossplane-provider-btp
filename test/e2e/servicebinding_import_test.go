//go:build e2e
// +build e2e

package e2e

import (
	"testing"
	"time"

	"sigs.k8s.io/e2e-framework/klient/wait"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

func TestServiceBindingImportFlow(t *testing.T) {
	importTester := NewImportTester(
		&v1alpha1.ServiceBinding{
			Spec: v1alpha1.ServiceBindingSpec{
				ForProvider: v1alpha1.ServiceBindingParameters{
					Name: "e2e-test-servicebinding",
					SubaccountRef:      &xpv1.Reference{
						Name: "e2e-test-servicebinding",
					},
					ServiceInstanceRef: &xpv1.Reference{
						Name: "e2e-destination-instance",
					},

				},
			},
		},
		"e2e-test-servicebinding-import",
		WithWaitCreateTimeout[*v1alpha1.ServiceBinding](wait.WithTimeout(15*time.Minute)),
		WithWaitDeletionTimeout[*v1alpha1.ServiceBinding](wait.WithTimeout(5*time.Minute)),
		WithDependentResourceDirectory[*v1alpha1.ServiceBinding]("testdata/crs/servicebinding/env"),
	)
	importFeature := importTester.BuildTestFeature("BTP ServiceBinding Import Flow").Feature()
	testenv.Test(t, importFeature)
}

func TestServiceBindingImportFlow_Rotation(t *testing.T) {
	importTester := NewImportTester(
		&v1alpha1.ServiceBinding{
			Spec: v1alpha1.ServiceBindingSpec{
				ForProvider: v1alpha1.ServiceBindingParameters{
					Name: "e2e-test-servicebinding",
					SubaccountRef:      &xpv1.Reference{
						Name: "e2e-test-servicebinding",
					},
					ServiceInstanceRef: &xpv1.Reference{
						Name: "e2e-destination-instance",
					},

				},
				Rotation: &v1alpha1.RotationParameters{
					Frequency: &metav1.Duration{5*time.Minute},
					TTL: &metav1.Duration{7*time.Minute},
				},
			},
		},
		"e2e-test-servicebinding-import-rotation",
		WithWaitCreateTimeout[*v1alpha1.ServiceBinding](wait.WithTimeout(15*time.Minute)),
		WithWaitDeletionTimeout[*v1alpha1.ServiceBinding](wait.WithTimeout(5*time.Minute)),
		WithDependentResourceDirectory[*v1alpha1.ServiceBinding]("testdata/crs/servicebinding/env"),
	)
	importFeature := importTester.BuildTestFeature("BTP ServiceBinding Import Flow").Feature()
	testenv.Test(t, importFeature)
}

