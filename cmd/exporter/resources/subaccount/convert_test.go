package subaccount

import (
	"testing"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	openapiaccount "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

func TestConvertSubaccountResource(t *testing.T) {
	r := require.New(t)

	tests := []struct {
		name                string
		subaccount          *openapiaccount.SubaccountResponseObject
		wantName            string
		wantExternalName    string
		wantDisplayName     string
		wantRegion          string
		wantSubdomain       string
		wantAdmins          []string
		wantGlobalAccount   string
		wantDirectory       string
		wantDescription     string
		wantUsedForProd     string
		wantBetaEnabled     bool
		wantLabels          map[string][]string
		wantCommented       bool
		wantCommentContains []string
	}{
		{
			name: "all required fields present",
			subaccount: &openapiaccount.SubaccountResponseObject{
				DisplayName: "Test Subaccount",
				Guid:        "12345678-1234-1234-1234-123456789abc",
				Region:      "eu10",
				Subdomain:   "test-subdomain",
				CreatedBy:   ptr.To("test@example.com"),
			},
			wantName:         "test-subaccount",
			wantExternalName: "12345678-1234-1234-1234-123456789abc",
			wantDisplayName:  "Test Subaccount",
			wantRegion:       "eu10",
			wantSubdomain:    "test-subdomain",
			wantAdmins:       []string{"test@example.com"},
			wantCommented:    false,
		},
		{
			name: "all fields including optional",
			subaccount: &openapiaccount.SubaccountResponseObject{
				DisplayName:       "Full Subaccount",
				Guid:              "87654321-4321-4321-4321-cba987654321",
				Region:            "us10",
				Subdomain:         "full-subdomain",
				CreatedBy:         ptr.To("admin@example.com"),
				GlobalAccountGUID: "global-account-guid",
				ParentGUID:        "directory-guid",
				Description:       "Test description",
				UsedForProduction: "USED_FOR_PRODUCTION",
				BetaEnabled:       true,
				Labels:            &map[string][]string{"env": {"dev"}, "team": {"platform"}},
			},
			wantName:          "full-subaccount",
			wantExternalName:  "87654321-4321-4321-4321-cba987654321",
			wantDisplayName:   "Full Subaccount",
			wantRegion:        "us10",
			wantSubdomain:     "full-subdomain",
			wantAdmins:        []string{"admin@example.com"},
			wantGlobalAccount: "global-account-guid",
			wantDirectory:     "directory-guid",
			wantDescription:   "Test description",
			wantUsedForProd:   "USED_FOR_PRODUCTION",
			wantBetaEnabled:   true,
			wantLabels:        map[string][]string{"env": {"dev"}, "team": {"platform"}},
			wantCommented:     false,
		},
		{
			name: "missing displayName",
			subaccount: &openapiaccount.SubaccountResponseObject{
				Guid:      "12345678-1234-1234-1234-123456789abc",
				Region:    "eu10",
				Subdomain: "test-subdomain",
				CreatedBy: ptr.To("test@example.com"),
			},
			wantName:            "12345678-1234-1234-1234-123456789abc",
			wantExternalName:    "12345678-1234-1234-1234-123456789abc",
			wantDisplayName:     "",
			wantRegion:          "eu10",
			wantSubdomain:       "test-subdomain",
			wantAdmins:          []string{"test@example.com"},
			wantCommented:       true,
			wantCommentContains: []string{"WARNING: 'displayName' field is missing"},
		},
		{
			name: "missing guid",
			subaccount: &openapiaccount.SubaccountResponseObject{
				DisplayName: "Test Subaccount",
				Region:      "eu10",
				Subdomain:   "test-subdomain",
				CreatedBy:   ptr.To("test@example.com"),
			},
			wantName:            "test-subaccount",
			wantExternalName:    "",
			wantDisplayName:     "Test Subaccount",
			wantRegion:          "eu10",
			wantSubdomain:       "test-subdomain",
			wantAdmins:          []string{"test@example.com"},
			wantCommented:       true,
			wantCommentContains: []string{"WARNING: 'guid' field is missing"},
		},
		{
			name: "missing region",
			subaccount: &openapiaccount.SubaccountResponseObject{
				DisplayName: "Test Subaccount",
				Guid:        "12345678-1234-1234-1234-123456789abc",
				Subdomain:   "test-subdomain",
				CreatedBy:   ptr.To("test@example.com"),
			},
			wantName:            "test-subaccount",
			wantExternalName:    "12345678-1234-1234-1234-123456789abc",
			wantDisplayName:     "Test Subaccount",
			wantRegion:          "",
			wantSubdomain:       "test-subdomain",
			wantAdmins:          []string{"test@example.com"},
			wantCommented:       true,
			wantCommentContains: []string{"WARNING: 'region' field is missing"},
		},
		{
			name: "missing subdomain",
			subaccount: &openapiaccount.SubaccountResponseObject{
				DisplayName: "Test Subaccount",
				Guid:        "12345678-1234-1234-1234-123456789abc",
				Region:      "eu10",
				CreatedBy:   ptr.To("test@example.com"),
			},
			wantName:            "test-subaccount",
			wantExternalName:    "12345678-1234-1234-1234-123456789abc",
			wantDisplayName:     "Test Subaccount",
			wantRegion:          "eu10",
			wantSubdomain:       "",
			wantAdmins:          []string{"test@example.com"},
			wantCommented:       true,
			wantCommentContains: []string{"WARNING: 'subdomain' field is missing"},
		},
		{
			name: "missing createdBy",
			subaccount: &openapiaccount.SubaccountResponseObject{
				DisplayName: "Test Subaccount",
				Guid:        "12345678-1234-1234-1234-123456789abc",
				Region:      "eu10",
				Subdomain:   "test-subdomain",
			},
			wantName:            "test-subaccount",
			wantExternalName:    "12345678-1234-1234-1234-123456789abc",
			wantDisplayName:     "Test Subaccount",
			wantRegion:          "eu10",
			wantSubdomain:       "test-subdomain",
			wantAdmins:          []string{""},
			wantCommented:       true,
			wantCommentContains: []string{"WARNING: 'createdBy' field is missing"},
		},
		{
			name: "parentGUID same as globalAccountGUID",
			subaccount: &openapiaccount.SubaccountResponseObject{
				DisplayName:       "Test Subaccount",
				Guid:              "12345678-1234-1234-1234-123456789abc",
				Region:            "eu10",
				Subdomain:         "test-subdomain",
				CreatedBy:         ptr.To("test@example.com"),
				GlobalAccountGUID: "ga-guid",
				ParentGUID:        "ga-guid",
			},
			wantName:          "test-subaccount",
			wantExternalName:  "12345678-1234-1234-1234-123456789abc",
			wantDisplayName:   "Test Subaccount",
			wantRegion:        "eu10",
			wantSubdomain:     "test-subdomain",
			wantAdmins:        []string{"test@example.com"},
			wantGlobalAccount: "ga-guid",
			wantDirectory:     "",
			wantCommented:     false,
		},
		{
			name: "special characters in displayName",
			subaccount: &openapiaccount.SubaccountResponseObject{
				DisplayName: "Test_Subaccount With Spaces & Special!@#",
				Guid:        "12345678-1234-1234-1234-123456789abc",
				Region:      "eu10",
				Subdomain:   "test-subdomain",
				CreatedBy:   ptr.To("test@example.com"),
			},
			wantName:         "test-subaccount-with-spaces--special",
			wantExternalName: "12345678-1234-1234-1234-123456789abc",
			wantDisplayName:  "Test_Subaccount With Spaces & Special!@#",
			wantRegion:       "eu10",
			wantSubdomain:    "test-subdomain",
			wantAdmins:       []string{"test@example.com"},
			wantCommented:    false,
		},
		{
			name: "empty string fields",
			subaccount: &openapiaccount.SubaccountResponseObject{
				DisplayName: "",
				Guid:        "",
				Region:      "",
				Subdomain:   "",
				CreatedBy:   ptr.To(""),
			},
			wantName:         "",
			wantExternalName: "",
			wantDisplayName:  "",
			wantRegion:       "",
			wantSubdomain:    "",
			wantAdmins:       []string{""},
			wantCommented:    true,
			wantCommentContains: []string{
				"WARNING: 'displayName' field is missing",
				"WARNING: 'guid' field is missing",
				"WARNING: 'region' field is missing",
				"WARNING: 'subdomain' field is missing",
				"WARNING: 'createdBy' field is missing",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertSubaccountResource(tt.subaccount)
			r.NotNil(result)

			// Check comment status
			comment, hasComment := result.Comment()
			r.Equal(tt.wantCommented, hasComment)

			// Check comment contents
			if tt.wantCommented {
				for _, substr := range tt.wantCommentContains {
					r.Contains(comment, substr)
				}
			}

			// Get the underlying Subaccount resource
			sa, ok := result.Resource().(*v1alpha1.Subaccount)
			r.Truef(ok, "Resource must be of type v1alpha1.Subaccount")

			// Verify metadata
			r.Equal(v1alpha1.SubaccountKind, sa.TypeMeta.Kind)
			r.Equal(v1alpha1.CRDGroupVersion.String(), sa.TypeMeta.APIVersion)
			r.Equal(tt.wantName, sa.ObjectMeta.Name)
			r.Equal(tt.wantExternalName, sa.ObjectMeta.Annotations["crossplane.io/external-name"])

			// Verify ManagementPolicies
			r.Equal(1, len(sa.Spec.ManagementPolicies))
			r.Equal(v1.ManagementActionObserve, sa.Spec.ManagementPolicies[0])

			// Verify providerConfigRef
			r.Nil(sa.GetProviderConfigReference(), "providerConfigRef must not be set")

			// Verify required fields
			r.Equal(tt.wantDisplayName, sa.Spec.ForProvider.DisplayName)
			r.Equal(tt.wantRegion, sa.Spec.ForProvider.Region)
			r.Equal(tt.wantSubdomain, sa.Spec.ForProvider.Subdomain)
			r.NotNil(sa.Spec.ForProvider.SubaccountAdmins, "SubaccountAdmins must not be nil")
			r.Equal(tt.wantAdmins, sa.Spec.ForProvider.SubaccountAdmins)

			// Verify optional fields
			r.Equal(tt.wantGlobalAccount, sa.Spec.ForProvider.GlobalAccountGuid)
			r.Equal(tt.wantDirectory, sa.Spec.ForProvider.DirectoryGuid)
			r.Equal(tt.wantDescription, sa.Spec.ForProvider.Description)
			r.Equal(tt.wantUsedForProd, sa.Spec.ForProvider.UsedForProduction)
			r.Equal(tt.wantBetaEnabled, sa.Spec.ForProvider.BetaEnabled)
			r.Equal(tt.wantLabels, sa.Spec.ForProvider.Labels)
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || len(s) > len(substr)+1 && findInString(s, substr)))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
