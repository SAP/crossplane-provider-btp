package subaccount

import (
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/yaml"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	openapiaccount "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

const (
	warnMissingDisplayName = "WARNING: 'displayName' field is missing in the source, cannot set 'DisplayName'"
	warnMissingGuid        = "WARNING: 'guid' field is missing in the source, cannot set 'external-name'"
	warnMissingRegion      = "WARNING: 'region' field is missing in the source, cannot set 'Region'"
	warnMissingSubdomain   = "WARNING: 'subdomain' field is missing in the source, cannot set 'Subdomain'"
	warnMissingCreatedBy   = "WARNING: 'createdBy' field is missing in the source, cannot set 'SubaccountAdmins'"
)

// convertSubaccountResource converts the given OpenAPI subaccount response object to a Subaccount custom resource,
// defined by the BTP Crossplane provider.
// - The function fills all mandatory fields
// - The function comments out the resource, if any of the mandatory fields is missing
// - The function fills optional fields that are relevant for the Update operation
// - The function does not perform extensive input validation, e.g. if an uuid string is a valid uuid.
func convertSubaccountResource(subaccount *openapiaccount.SubaccountResponseObject) *yaml.ResourceWithComment {
	saDisplayName, hasName := resources.StringValueOk(subaccount.GetDisplayNameOk())
	saGuid, hasGuid := resources.StringValueOk(subaccount.GetGuidOk())
	saRegion, hasRegion := resources.StringValueOk(subaccount.GetRegionOk())
	saSubdomain, hasSubdomain := resources.StringValueOk(subaccount.GetSubdomainOk())
	saCreatedBy, hasCreatedBy := resources.StringValueOk(subaccount.GetCreatedByOk())

	// Create Subaccount with required fields first.
	saResource := yaml.NewResourceWithComment(
		&v1alpha1.Subaccount{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha1.SubaccountKind,
				APIVersion: v1alpha1.CRDGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				// TODO: switch to `exporttool/parsan` for name sanitization, once it supports RFC 1123.
				Name: resources.SanitizeK8sResourceName(saDisplayName, saGuid),
				Annotations: map[string]string{
					"crossplane.io/external-name": saGuid,
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

	// Comment the resource out, if any of the required fields is missing.
	if !hasName {
		saResource.AddComment(warnMissingDisplayName)
	}
	if !hasGuid {
		saResource.AddComment(warnMissingGuid)
	}
	if !hasRegion {
		saResource.AddComment(warnMissingRegion)
	}
	if !hasSubdomain {
		saResource.AddComment(warnMissingSubdomain)
	}
	if !hasCreatedBy {
		saResource.AddComment(warnMissingCreatedBy)
	}

	// Fill the optional fields that are relevant for the Update operation, to have it match status.atProvider
	// and not trigger an update right after managementPolicies is set to manage the resource.
	ga, ok := resources.StringValueOk(subaccount.GetGlobalAccountGUIDOk())
	if ok {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.GlobalAccountGuid = ga
	}
	parent, ok := resources.StringValueOk(subaccount.GetParentGUIDOk())
	if ok && parent != ga {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.DirectoryGuid = parent
	}
	v, ok := resources.StringValueOk(subaccount.GetDescriptionOk())
	if ok {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.Description = v
	}
	v, ok = resources.StringValueOk(subaccount.GetUsedForProductionOk())
	if ok {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.UsedForProduction = v
	}
	l, ok := subaccount.GetLabelsOk()
	if ok {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.Labels = *l
	}
	b, ok := resources.BoolValueOk(subaccount.GetBetaEnabledOk())
	if ok {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.BetaEnabled = b
	}

	return saResource
}
