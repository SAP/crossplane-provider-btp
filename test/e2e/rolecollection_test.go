//go:build e2e
// +build e2e

package e2e

import (
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"sigs.k8s.io/e2e-framework/klient/wait"
)

const (
	roleCollectionImportK8sResName = "e2e-test-rolecollection-import"
	subaccountApiCredRefName       = "e2e-rolecollection-import-xsuaa-cred"
)

// TestRoleCollectionImportFlow tests the import flow for RoleCollection resource
// according to the External Name Handling ADR.
//
// This test verifies that:
// 1. A RoleCollection can be created with dependencies (Subaccount + SubaccountApiCredential)
// 2. The external-name is properly set to the role collection name
// 3. The resource can be imported using the external-name annotation
// 4. Imported resources transition to healthy state
// 5. Imported resources can be observed and updated properly
func TestRoleCollectionImportFlow(t *testing.T) {
	importTester := NewImportTester(
		&v1alpha1.RoleCollection{
			Spec: v1alpha1.RoleCollectionSpec{
				ForProvider: v1alpha1.RoleCollectionParameters{
					Name:        roleCollectionImportK8sResName,
					Description: stringPtr("E2E test role collection for import flow"),
					RoleReferences: []v1alpha1.RoleReference{
						{
							RoleTemplateAppId: "cis-local!b14",
							RoleTemplateName:  "Subaccount_Viewer",
							Name:              "Subaccount Viewer",
						},
					},
				},
				XSUAACredentialsReference: v1alpha1.XSUAACredentialsReference{
					SubaccountApiCredentialRef: &xpv1.Reference{
						Name: subaccountApiCredRefName,
					},
				},
			},
		},
		roleCollectionImportK8sResName,
		WithWaitCreateTimeout[*v1alpha1.RoleCollection](wait.WithInterval(5*time.Minute)),
		WithWaitDeletionTimeout[*v1alpha1.RoleCollection](wait.WithInterval(3*time.Minute)),
		WithDependentResourceDirectory[*v1alpha1.RoleCollection]("./testdata/crs/rolecollection_import"),
		WithWaitDependentResourceTimeout[*v1alpha1.RoleCollection](wait.WithInterval(5*time.Minute)),
	)

	testenv.Test(
		t,
		importTester.BuildTestFeature("RoleCollection Import Flow").Feature(),
	)
}

func stringPtr(s string) *string {
	return &s
}
