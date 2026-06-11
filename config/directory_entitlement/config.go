package subaccount_trust_configuration

import (
	"github.com/crossplane/upjet/v2/pkg/config"
)

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_directory_entitlement", func(r *config.Resource) {
		r.ShortGroup = "account"
		r.Kind = "DirectoryEntitlement"

		r.References["directory_id"] = config.Reference{
			Type:              "github.com/sap/crossplane-provider-btp/apis/cluster/account/v1alpha1.Directory",
			Extractor:         "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.DirectoryUuid()",
			RefFieldName:      "DirectoryRef",
			SelectorFieldName: "DirectorySelector",
		}
	})
}
