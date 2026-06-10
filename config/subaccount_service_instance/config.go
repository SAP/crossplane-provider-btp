package subaccount_service_instance

import (
	"github.com/crossplane/upjet/v2/pkg/config"
)

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_subaccount_service_instance", func(r *config.Resource) {
		r.ShortGroup = "account"
		r.Kind = "SubaccountServiceInstance"

		// The BTP provider implements ResourceWithIdentity for this resource.
		// The "id" attribute is computed by the API; "NOT_EMPTY_GUID" is the
		// placeholder used for initial read calls before the real ID is known.
		r.ExternalName = config.FrameworkResourceWithComputedIdentifier("id", "NOT_EMPTY_GUID")
		r.ExternalName.OmittedFields = []string{"timeouts"}

		// note: can be overwritten during initialization
		r.UseAsync = true

		// we only use this resource internally, so there is no harm in avoiding usage of secrets here it makes the setup a lot easier
		r.TerraformResource.Schema["parameters"].Sensitive = false

		// ADR: disable external-name initialization
		r.ExternalName.DisableNameInitializer = true

	})
}
