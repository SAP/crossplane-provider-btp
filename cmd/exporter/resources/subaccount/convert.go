package subaccount

import (
	"regexp"
	"strings"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	openapiaccount "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

// convertSubaccountResource converts the given OpenAPI subaccount response object to a Subaccount custom resource,
// defined by the BTP Crossplane provider.
// - The function takes mandatory fields only
// - The does not perform input validation, i.e. assumes that all fields are set properly.
func convertSubaccountResource(subaccount *openapiaccount.SubaccountResponseObject) *v1alpha1.Subaccount {
	return &v1alpha1.Subaccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.SubaccountKind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			// TODO: switch to `exporttool/parsan` for name sanitization, once it supports RFC 1123.
			Name: sanitizeK8sResourceName(subaccount.DisplayName, subaccount.Guid),
			Annotations: map[string]string{
				"crossplane.io/external-name": subaccount.Guid,
			},
		},
		Spec: v1alpha1.SubaccountSpec{
			ResourceSpec: v1.ResourceSpec{
				ManagementPolicies: []v1.ManagementAction{
					v1.ManagementActionObserve,
				},
			},
			ForProvider: v1alpha1.SubaccountParameters{
				// Take over the required fields only.
				DisplayName: subaccount.DisplayName,
				Region:      subaccount.Region,
				// TODO: API doesn't seem to  provide the current list of admins? Just the creator?
				SubaccountAdmins: []string{*subaccount.CreatedBy},
				Subdomain:        subaccount.Subdomain,
			},
		},
	}
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
