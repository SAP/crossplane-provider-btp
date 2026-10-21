package globalaccount_trust_configuration

import (
	"github.com/crossplane/upjet/v2/pkg/config"
)

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_globalaccount_trust_configuration", func(r *config.Resource) {
		r.ShortGroup = "security"
		r.Kind = "GlobalaccountTrustConfiguration"

		// If an external-name is present, set the origin as it is required for TF's read call used in Read /
		// refresh / import but is optional in the spec.
		//
		// external-name is set from id which is documented to be equal to origin:
		// https://registry.terraform.io/providers/SAP/btp/latest/docs/resources/globalaccount_trust_configuration#id-1
		r.ExternalName.SetIdentifierArgumentFn = func(base map[string]any, externalName string) {
			if externalName != "" {
				base["origin"] = externalName
			}
		}
	})
}
