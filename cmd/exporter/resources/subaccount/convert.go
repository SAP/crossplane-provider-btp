package subaccount

import (
	"regexp"
	"strings"

	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/yaml"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
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
	saDisplayName, hasName := stringValueOk(subaccount.GetDisplayNameOk())
	saGuid, hasGuid := stringValueOk(subaccount.GetGuidOk())
	saRegion, hasRegion := stringValueOk(subaccount.GetRegionOk())
	saSubdomain, hasSubdomain := stringValueOk(subaccount.GetSubdomainOk())
	saCreatedBy, hasCreatedBy := stringValueOk(subaccount.GetCreatedByOk())

	// Create Subaccount with required fields first.
	saResource := yaml.NewResourceWithComment(
		&v1alpha1.Subaccount{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha1.SubaccountKind,
				APIVersion: v1alpha1.CRDGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				// TODO: switch to `exporttool/parsan` for name sanitization, once it supports RFC 1123.
				Name: sanitizeK8sResourceName(saDisplayName, saGuid),
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
	ga, ok := stringValueOk(subaccount.GetGlobalAccountGUIDOk())
	if ok {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.GlobalAccountGuid = ga
	}
	parent, ok := stringValueOk(subaccount.GetParentGUIDOk())
	if ok && parent != ga {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.DirectoryGuid = parent
	}
	v, ok := stringValueOk(subaccount.GetDescriptionOk())
	if ok {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.Description = v
	}
	v, ok = stringValueOk(subaccount.GetUsedForProductionOk())
	if ok {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.UsedForProduction = v
	}
	l, ok := subaccount.GetLabelsOk()
	if ok {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.Labels = *l
	}
	b, ok := boolValueOk(subaccount.GetBetaEnabledOk())
	if ok {
		saResource.Resource().(*v1alpha1.Subaccount).Spec.ForProvider.BetaEnabled = b
	}

	return saResource
}

func sanitizeK8sResourceName(s, fallback string) string {
	// Convert to lowercase.
	s = strings.ToLower(s)

	// Replace spaces and underscores with hyphens.
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")

	// Remove invalid characters (keep only alphanumeric and hyphens).
	reg := regexp.MustCompile("[^a-z0-9-]")
	s = reg.ReplaceAllString(s, "")

	// Remove leading/trailing hyphens.
	s = strings.Trim(s, "-")

	// Handle empty result.
	if s == "" {
		s = fallback
	}

	// Truncate to max length.
	if len(s) > validation.DNS1123LabelMaxLength { // 63 chars
		s = s[:validation.DNS1123LabelMaxLength]
	}

	// Ensure it doesn't end with hyphen after truncation.
	s = strings.TrimRight(s, "-")

	// Validate it.
	if errs := validation.IsDNS1123Label(s); len(errs) > 0 {
		// Handle validation errors if needed
		return fallback
	}

	return s
}

func stringValueOk(s *string, hint bool) (string, bool) {
	if !hint || s == nil {
		return "", false
	}
	if len(*s) == 0 {
		return "", false
	}
	return *s, true
}

func boolValueOk(b *bool, hint bool) (bool, bool) {
	if !hint || b == nil {
		return false, false
	}
	return *b, true
}
