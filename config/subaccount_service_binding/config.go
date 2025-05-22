package subaccount_trust_configuration

import (
	"context"

	"github.com/crossplane/upjet/pkg/config"
)

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_subaccount_service_binding", func(r *config.Resource) {
		r.ShortGroup = "account"
		r.Kind = "SubaccountServiceBinding"
		r.ExternalName.GetIDFn = func(_ context.Context, externalName string, _ map[string]any, _ map[string]any) (string, error) {
			// When using "" as ID the API endpoint call will fail, so we need to use anything else that
			// won't yield a result
			if externalName == "" {
				return "NOT_EMPTY_GUID", nil
			}
			return externalName, nil
		}
		// this prevents a callback to the manager, which makes integration of controller calls from within another controller easier
		r.UseAsync = true

		r.References["subaccount_id"] = config.Reference{
			Type:              "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount",
			Extractor:         "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()",
			RefFieldName:      "SubaccountRef",
			SelectorFieldName: "SubaccountSelector",
		}
		r.References["service_instance_id"] = config.Reference{
			Type:              "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.ServiceInstance",
			Extractor:         "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.ServiceInstanceUuid()",
			RefFieldName:      "ServiceInstanceRef",
			SelectorFieldName: "ServiceInstanceSelector",
		}
	})
}
