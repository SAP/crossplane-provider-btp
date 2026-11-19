package rolecollection

import (
	"context"

	"github.com/SAP/xp-clifford/yaml"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func convertRoleCollectionResource(ctx context.Context, rc *rolecollection) *yaml.ResourceWithComment {
	resourceName := rc.GenerateK8sResourceName()
	externalName := rc.GetExternalName()
	roleReferences := rc.roleCollection.RoleReferences

	// Leave Description nil when empty so omitempty drops it from the emitted
	// YAML rather than writing an explicit `description: ""`.
	var description *string
	if rc.roleCollection.Description != "" {
		description = &rc.roleCollection.Description
	}

	spec := v1alpha1.RoleCollectionSpec{
		ClusterManagedResourceSpec: xpv1.ClusterManagedResourceSpec{
			ManagementPolicies: []xpv1.ManagementAction{xpv1.ManagementActionObserve},
		},
		ForProvider: v1alpha1.RoleCollectionParameters{
			Name:           rc.roleCollection.Name,
			Description:    description,
			RoleReferences: roleReferences,
		},
	}

	// A RoleCollection needs xsuaa credentials for its parent subaccount, supplied
	// either inline via apiCredentials or by reference via subaccountApiCredentialRef
	// (exactly one). The exporter emits neither, so the reference is left empty and
	// the TODO comment below tells the user to supply one, keeping the resource
	// commented out until they do.
	spec.SubaccountApiCredentialRef = &xpv1.Reference{Name: ""}

	roleCollection := yaml.NewResourceWithComment(&v1alpha1.RoleCollection{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.RoleCollectionKind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: resourceName,
			Annotations: map[string]string{
				"crossplane.io/external-name": externalName,
			},
		},
		Spec: spec,
	})
	roleCollection.CloneComment(rc)

	roleCollection.AddComment("TODO: provide xsuaa credentials for this subaccount, either by setting apiCredentials or by adding a subaccountApiCredentialRef to a SubaccountApiCredential (readOnly: false); neither is exported")

	if resourceName == resources.UndefinedName {
		roleCollection.AddComment(resources.WarnUndefinedResourceName)
	}
	if externalName == resources.UndefinedExternalName {
		roleCollection.AddComment(resources.WarnUndefinedExternalName)
	}
	return roleCollection
}
