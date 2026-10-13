package subaccount_service_binding

import (
	"github.com/crossplane/upjet/v2/pkg/config"
)

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_subaccount_service_binding", func(r *config.Resource) {
		r.ShortGroup = "account"
		r.Kind = "SubaccountServiceBinding"

		// Writes "NOT_EMPTY_GUID" as "id" while the external-name is empty, so the
		// pre-create Read fails instead of matching an arbitrary binding. Returns a
		// fresh ExternalName, so the fields set below must follow it.
		r.ExternalName = config.FrameworkResourceWithComputedIdentifier("id", "NOT_EMPTY_GUID")

		r.ExternalName.DisableNameInitializer = true

		// note: can be overwritten during initialization
		r.UseAsync = true

		// issue #962: avoid late initialization trigger. parameters is
		// Optional+Computed, so late-init would rewrite the spec and short-circuit
		// Observe before Plan on a parameters-only update.
		r.LateInitializer.IgnoredFields = []string{"parameters"}
	})
}
