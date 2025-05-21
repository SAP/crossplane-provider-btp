package btp_subaccount_api_credential

import (
	"github.com/crossplane/upjet/pkg/config"
)

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_subaccount_api_credential", func(r *config.Resource) {
		r.ShortGroup = "security"
		r.Kind = "SubaccountApiCredential"
		r.UseAsync = false

		// Mark all as sensitive to exclude them from the status
		r.TerraformResource.Schema["client_secret"].Sensitive = true
		r.TerraformResource.Schema["client_id"].Sensitive = true
		r.TerraformResource.Schema["token_url"].Sensitive = true
		r.TerraformResource.Schema["api_url"].Sensitive = true

		r.ExternalName.SetIdentifierArgumentFn = func(base map[string]any, name string) {
			if name == "" {
				base["name"] = "managed-subbaccount-api-credential"
			} else {
				base["name"] = name
			}
		}
		r.ExternalName.GetExternalNameFn = func(tfstate map[string]any) (string, error) {
			return tfstate["name"].(string), nil
		}

		r.References["subaccount_id"] = config.Reference{
			Type:              "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount",
			Extractor:         "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()",
			RefFieldName:      "SubaccountRef",
			SelectorFieldName: "SubaccountSelector",
		}
	})
}
