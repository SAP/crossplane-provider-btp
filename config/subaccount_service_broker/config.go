package subaccountservicebroker

import (
	"context"

	"github.com/crossplane/upjet/pkg/config"
)

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_subaccount_service_broker", func(r *config.Resource) {
		r.ShortGroup = "account"
		r.Kind = "SubaccountServiceBroker"

		r.ExternalName.GetIDFn = func(_ context.Context, externalName string, _ map[string]any, _ map[string]any) (string, error) {
			// When using "" as ID the API endpoint call will fail, so we need to use anything else that
			// won't yield a result
			if externalName == "" {
				return "NOT_EMPTY_GUID", nil
			}
			return externalName, nil
		}

		r.References["subaccount_id"] = config.Reference{
			Type:              "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount",
			Extractor:         "github.com/crossplane/crossplane-runtime/pkg/reference.ExternalName()",
			RefFieldName:      "SubaccountRef",
			SelectorFieldName: "SubaccountSelector",
		}

	})
}
