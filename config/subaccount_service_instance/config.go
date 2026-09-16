package subaccount_service_instance

import (
	"github.com/crossplane/upjet/v2/pkg/config"
)

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_subaccount_service_instance", func(r *config.Resource) {
		r.ShortGroup = "account"
		r.Kind = "SubaccountServiceInstance"

		// issue #691: reconciled in-process by the Terraform Plugin Framework
		// client. The helper writes "NOT_EMPTY_GUID" as "id" while the
		// external-name is empty, so the pre-create Read fails instead of
		// matching an arbitrary instance. It returns a fresh ExternalName, so
		// the fields customised below must be re-applied after it.
		r.ExternalName = config.FrameworkResourceWithComputedIdentifier("id", "NOT_EMPTY_GUID")

		// Must never become []string{"timeouts"}: the mapper populates
		// spec.forProvider.timeouts from operationTimeout (issue #699), so
		// timeouts must stay in the spec.
		r.ExternalName.OmittedFields = []string{}

		// ADR: disable external-name initialization
		// Re-applied: the assignment above returns a fresh ExternalName.
		r.ExternalName.DisableNameInitializer = true

		// note: can be overwritten during initialization
		r.UseAsync = true

		// we only use this resource internally, so there is no harm in avoiding usage of secrets here it makes the setup a lot easier
		r.TerraformResource.Schema["parameters"].Sensitive = false

		// issue #962: avoid late initialization trigger. Optional+Computed fields
		// cause late-init to rewrite the spec and short-circuit Observe before
		// Plan on a parameters-only update.
		r.LateInitializer.IgnoredFields = []string{
			"service_offering_name", "serviceplan_name", "serviceplan_id", "parameters",
		}
	})
}
