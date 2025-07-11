package subaccount_service_instance

import (
	"github.com/crossplane/upjet/pkg/config"
)

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_subaccount", func(r *config.Resource) {
		r.ShortGroup = "account"
		r.Kind = "TfSubaccount"

		r.UseAsync = true

	})
}
