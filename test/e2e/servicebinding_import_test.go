//go:build e2e
// +build e2e

package e2e

import (
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"sigs.k8s.io/e2e-framework/klient/wait"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

// TestServiceBindingImportFlow proves the external-name contract end to end:
// create a binding, capture its GUID, delete the CR (leaving the BTP binding),
// then re-import it via crossplane.io/external-name and assert it becomes
// healthy. The dependent subaccount/servicemanager/serviceinstance are applied
// from the servicebinding_import fixture dir.
func TestServiceBindingImportFlow(t *testing.T) {
	importTester := NewImportTester(
		&v1alpha1.ServiceBinding{
			Spec: v1alpha1.ServiceBindingSpec{
				ForProvider: v1alpha1.ServiceBindingParameters{
					Name:               "e2e-destination-binding-import",
					ServiceInstanceRef: &xpv1.Reference{Name: "e2e-servicebinding-import-instance"},
					SubaccountRef:      &xpv1.Reference{Name: "e2e-test-servicebinding-import"},
				},
				ResourceSpec: xpv1.ResourceSpec{
					WriteConnectionSecretToReference: &xpv1.SecretReference{
						Name:      "e2e-destination-binding-import",
						Namespace: "default",
					},
				},
			},
		},
		"e2e-destination-binding-import",
		WithWaitDependentResourceTimeout[*v1alpha1.ServiceBinding](wait.WithTimeout(20*time.Minute)),
		WithWaitCreateTimeout[*v1alpha1.ServiceBinding](wait.WithTimeout(20*time.Minute)),
		WithWaitDeletionTimeout[*v1alpha1.ServiceBinding](wait.WithTimeout(20*time.Minute)),
		WithDependentResourceDirectory[*v1alpha1.ServiceBinding](crsPath("servicebinding_import")),
	)

	importFeature := importTester.BuildTestFeature("BTP ServiceBinding Import Flow").Feature()
	testenv.Test(t, importFeature)
}
