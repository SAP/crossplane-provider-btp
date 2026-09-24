package subaccountservicebroker

import (
	"context"
	"fmt"
	"strings"

	"github.com/crossplane/upjet/v2/pkg/config"
	"github.com/pkg/errors"
)

// notEmptyGUID is returned when the external-name is empty: using "" as ID
// would make the API endpoint call fail, so we use a value that never matches.
const notEmptyGUID = "NOT_EMPTY_GUID"

// getBrokerID translates the external-name annotation into the Terraform
// resource id.
// ADR(external-name): the annotation holds the compound key
// "<subaccount-id>/<broker-id>", but this function must return only the broker
// id: the BTP provider's Read locates the broker via the bare `id` state
// attribute, and upjet writes this function's result into that attribute on
// every fresh state build (upjet EnsureTFState). Returning the compound key
// would break every reconcile after a provider restart. As a consequence,
// observe-only imports (terraform import needs "<subaccount-id>,<broker-id>")
// are not supported for this resource — import via managementPolicies: ["*"].
// Legacy bare-GUID annotations pass through unchanged and are migrated to the
// compound format by getBrokerExternalName on the next Observe.
func getBrokerID(_ context.Context, externalName string, _ map[string]any, _ map[string]any) (string, error) {
	if externalName == "" {
		return notEmptyGUID, nil
	}
	if strings.Contains(externalName, "/") {
		parts := strings.Split(externalName, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", errors.Errorf("invalid compound external-name %q: expected \"<subaccount-id>/<broker-id>\"", externalName)
		}
		return parts[1], nil
	}
	return externalName, nil
}

// getBrokerExternalName reconstructs the ADR compound key
// "<subaccount-id>/<broker-id>" (ADR delimiter: "/") from TF state after
// Create/Observe. This also migrates legacy bare-GUID annotations on the
// first reconcile after upgrade.
func getBrokerExternalName(tfstate map[string]any) (string, error) {
	subaccountID, _ := tfstate["subaccount_id"].(string)
	id, _ := tfstate["id"].(string)
	if subaccountID == "" {
		return "", errors.New("cannot reconstruct external-name: subaccount_id missing from tfstate")
	}
	if id == "" {
		return "", errors.New("cannot reconstruct external-name: id missing from tfstate")
	}
	return fmt.Sprintf("%s/%s", subaccountID, id), nil
}

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_subaccount_service_broker", func(r *config.Resource) {
		r.ShortGroup = "account"
		r.Kind = "SubaccountServiceBroker"

		r.ExternalName.GetIDFn = getBrokerID
		r.ExternalName.GetExternalNameFn = getBrokerExternalName

		r.References["subaccount_id"] = config.Reference{
			Type:              "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount",
			Extractor:         "github.com/crossplane/crossplane-runtime/v2/pkg/reference.ExternalName()",
			RefFieldName:      "SubaccountRef",
			SelectorFieldName: "SubaccountSelector",
		}
	})
}
