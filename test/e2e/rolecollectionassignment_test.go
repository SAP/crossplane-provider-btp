//go:build e2e
// +build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/crossplane-contrib/xp-testing/pkg/envvar"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"sigs.k8s.io/e2e-framework/klient/wait"
)

const (
	rcaImportK8sResName     = "e2e-test-rca-import"
	rcaImportApiCredRefName = "e2e-rca-import-xsuaa-cred"
	// rcaImportRoleCollection is a built-in role collection that exists in every BTP subaccount
	// out of the box, so the test does not need to provision one.
	rcaImportRoleCollection = "Subaccount Viewer"
	rcaImportOrigin         = "sap.default"
)

// TestRoleCollectionAssignmentImportFlow tests the import flow for
// RoleCollectionAssignment resource according to the External Name Handling ADR.
//
// This test verifies that:
//  1. A RoleCollectionAssignment can be created with dependencies (Subaccount + SubaccountApiCredential)
//     against the built-in "Subaccount Viewer" role collection
//  2. The external-name is set to the compound key "<origin>/<userOrGroup>/<roleCollection>"
//  3. The resource can be imported using the external-name annotation
//  4. Imported resources transition to healthy state
//  5. Imported resources can be observed and deleted properly
//
// Group flow is covered by unit tests only; this test exercises the user flow.
func TestRoleCollectionAssignmentImportFlow(t *testing.T) {
	importTester := NewImportTester(
		&v1alpha1.RoleCollectionAssignment{
			Spec: v1alpha1.RoleCollectionAssignmentSpec{
				ForProvider: v1alpha1.RoleCollectionAssignmentParameters{
					Origin:             rcaImportOrigin,
					UserName:           envvar.GetOrPanic(TECHNICAL_USER_EMAIL_ENV_KEY),
					RoleCollectionName: rcaImportRoleCollection,
				},
				XSUAACredentialsReference: v1alpha1.XSUAACredentialsReference{
					SubaccountApiCredentialRef: &xpv1.Reference{
						Name: rcaImportApiCredRefName,
					},
				},
			},
		},
		rcaImportK8sResName,
		WithWaitCreateTimeout[*v1alpha1.RoleCollectionAssignment](wait.WithTimeout(5*time.Minute)),
		WithWaitDeletionTimeout[*v1alpha1.RoleCollectionAssignment](wait.WithTimeout(3*time.Minute)),
		WithDependentResourceDirectory[*v1alpha1.RoleCollectionAssignment]("./testdata/crs/rolecollectionassignment_import"),
		WithWaitDependentResourceTimeout[*v1alpha1.RoleCollectionAssignment](wait.WithTimeout(15*time.Minute)),
	)

	testenv.Test(
		t,
		importTester.BuildTestFeature("RoleCollectionAssignment Import Flow").Feature(),
	)
}
