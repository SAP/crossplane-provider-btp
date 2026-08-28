package subaccount_trust_configuration

import (
	"context"
	"fmt"
	"strings"

	"github.com/crossplane/upjet/v2/pkg/config"
	"github.com/pkg/errors"
)

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_directory_entitlement", func(r *config.Resource) {
		r.ShortGroup = "account"
		r.Kind = "DirectoryEntitlement"

		r.References["directory_id"] = config.Reference{
			Type:              "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Directory",
			Extractor:         "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.DirectoryUuid()",
			RefFieldName:      "DirectoryRef",
			SelectorFieldName: "DirectorySelector",
		}

		// ADR(external-name): reconstruct compound key "directory_id/service_name/plan_name" (ADR delimiter: "/")
		// from TF state after Create/Observe so Upjet does not overwrite the annotation with just the plan ID.
		r.ExternalName.GetExternalNameFn = func(tfstate map[string]any) (string, error) {
			directoryID, _ := tfstate["directory_id"].(string)
			serviceName, _ := tfstate["service_name"].(string)
			planName, _ := tfstate["plan_name"].(string)
			if directoryID == "" {
				return "", errors.New("cannot reconstruct external-name: directory_id missing from tfstate")
			}
			if serviceName == "" {
				return "", errors.New("cannot reconstruct external-name: service_name missing from tfstate")
			}
			if planName == "" {
				return "", errors.New("cannot reconstruct external-name: plan_name missing from tfstate")
			}
			return fmt.Sprintf("%s/%s/%s", directoryID, serviceName, planName), nil
		}

		// ADR(external-name): translate ADR compound key "directory_id/service_name/plan_name" to TF import ID
		// format "directory_id,service_name,plan_name" required by the BTP Terraform provider's ImportState function.
		r.ExternalName.GetIDFn = func(_ context.Context, externalName string, _ map[string]any, _ map[string]any) (string, error) {
			parts := strings.SplitN(externalName, "/", 3)
			if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
				return externalName, nil
			}
			return fmt.Sprintf("%s,%s,%s", parts[0], parts[1], parts[2]), nil
		}
	})
}
