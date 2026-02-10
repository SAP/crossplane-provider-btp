package subaccount

import (
	"github.com/SAP/xp-clifford/yaml"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

const (
	warnMissingDisplayName = "WARNING: 'displayName' field is missing in the source, cannot set 'DisplayName'"
	warnMissingGuid        = "WARNING: 'guid' field is missing in the source, cannot set 'external-name'"
	warnMissingRegion      = "WARNING: 'region' field is missing in the source, cannot set 'Region'"
	warnMissingSubdomain   = "WARNING: 'subdomain' field is missing in the source, cannot set 'Subdomain'"
	warnMissingCreatedBy   = "WARNING: 'createdBy' field is missing in the source, cannot set 'SubaccountAdmins'"
)

func convertSubaccountResource(sa *subaccount) *yaml.ResourceWithComment {
	saDisplayName := sa.GetDisplayName()
	saGuid := sa.GetID()
	saRegion := sa.Region
	saSubdomain := sa.Subdomain
	saCreatedBy := sa.CreatedBy
	resourceName := sa.GenerateK8sResourceName()
	externalName := sa.GetExternalName()

	// Create Subaccount with required fields first.
	saResource := yaml.NewResourceWithComment(
		&v1alpha1.Subaccount{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha1.SubaccountKind,
				APIVersion: v1alpha1.CRDGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: resourceName,
				Annotations: map[string]string{
					"crossplane.io/external-name": externalName,
				},
			},
			Spec: v1alpha1.SubaccountSpec{
				ResourceSpec: v1.ResourceSpec{
					ManagementPolicies: []v1.ManagementAction{
						v1.ManagementActionObserve,
					},
				},
				ForProvider: v1alpha1.SubaccountParameters{
					DisplayName: saDisplayName,
					Region:      saRegion,
					// API doesn't provide the current list of admins. Using CreatedBy as the only admin for now.
					SubaccountAdmins: []string{saCreatedBy},
					Subdomain:        saSubdomain,
				},
			},
		})

	// Copy comments from the original resource.
	saResource.CloneComment(sa)

	// Comment the resource out, if any of the required fields is missing.
	if saDisplayName == "" {
		saResource.AddComment(warnMissingDisplayName)
	}
	if saGuid == "" {
		saResource.AddComment(warnMissingGuid)
	}
	if saRegion == "" {
		saResource.AddComment(warnMissingRegion)
	}
	if saSubdomain == "" {
		saResource.AddComment(warnMissingSubdomain)
	}
	if saCreatedBy == "" {
		saResource.AddComment(warnMissingCreatedBy)
	}

	// Fill the optional fields that are relevant for the Update operation, to have it match status.atProvider
	// and not trigger an update right after managementPolicies is set to manage the resource.

	// GlobalAccountGuid
	if sa.GlobalAccountGUID != "" {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.GlobalAccountGuid = sa.GlobalAccountGUID
	}

	// DirectoryGuid
	if sa.ParentGUID != "" && sa.ParentGUID != sa.GlobalAccountGUID {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.DirectoryGuid = sa.ParentGUID
	}

	// Description
	if sa.Description != "" {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.Description = sa.Description
	}

	// UsedForProduction
	if sa.UsedForProduction != "" {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.UsedForProduction = sa.UsedForProduction
	}

	// Labels
	if len(sa.Labels) > 0 {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.Labels = sa.Labels
	}

	// BetaEnabled
	saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.BetaEnabled = sa.BetaEnabled

	return saResource
}
