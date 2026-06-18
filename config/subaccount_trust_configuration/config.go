package subaccount_trust_configuration

import (
	"context"
	"fmt"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/upjet/v2/pkg/config"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

const errTypeAssertion = "managed resource is not of type SubaccountTrustConfiguration"

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_subaccount_trust_configuration", func(r *config.Resource) {
		r.ShortGroup = "security"
		r.Kind = "SubaccountTrustConfiguration"

		// Workaround for inconsistent description on create, see #724.
		// TODO: remove when terraform-provider-btp is bumped to >= 1.23.1 (#521).
		r.TerraformConfigurationInjector = func(jsonMap, tfMap map[string]any) error {
			delete(tfMap, "description")
			return nil
		}

		r.References["subaccount_id"] = config.Reference{
			Type:              "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount",
			Extractor:         "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()",
			RefFieldName:      "SubaccountRef",
			SelectorFieldName: "SubaccountSelector",
		}

		// ADR(external-name): reconstruct compound key "subaccount_id/origin" (ADR delimiter: "/") from
		// TF state after Create/Observe so Upjet does not overwrite the annotation with just the origin string.
		// Both fields are present in the TF state returned by the BTP provider.
		r.ExternalName.GetExternalNameFn = func(tfstate map[string]any) (string, error) {
			subaccountID, _ := tfstate["subaccount_id"].(string)
			origin, _ := tfstate["origin"].(string)
			if subaccountID == "" {
				return "", errors.New("cannot reconstruct external-name: subaccount_id missing from tfstate")
			}
			if origin == "" {
				return "", errors.New("cannot reconstruct external-name: origin missing from tfstate")
			}
			return fmt.Sprintf("%s/%s", subaccountID, origin), nil
		}

		// ADR(external-name): translate ADR compound key "subaccount_id/origin" to TF import ID
		// format "subaccount_id,origin" required by the BTP Terraform provider's ImportState function.
		r.ExternalName.GetIDFn = func(_ context.Context, externalName string, _ map[string]any, _ map[string]any) (string, error) {
			parts := strings.SplitN(externalName, "/", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return externalName, nil
			}
			return fmt.Sprintf("%s,%s", parts[0], parts[1]), nil
		}

		// ADR(external-name): seeds status.atProvider.origin from the external-name annotation
		// so that terraform refresh can locate the resource without triggering a spurious Create.
		r.InitializerFns = append(r.InitializerFns, func(kube client.Client) managed.Initializer {
			return &OriginInitializer{Kube: kube}
		})
	})
}

// OriginInitializer copies the origin portion of the compound external-name annotation
// ("subaccount_id/origin") into status.atProvider.origin when the annotation is set
// but the observation field is still empty.
type OriginInitializer struct {
	Kube client.Client
}

func (o *OriginInitializer) Initialize(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*securityv1alpha1.SubaccountTrustConfiguration)
	if !ok {
		return errors.New(errTypeAssertion)
	}

	externalName := meta.GetExternalName(cr)
	if externalName == "" || cr.Status.AtProvider.Origin != nil {
		return nil
	}

	// Parse compound key "<subaccount-id>/<origin>" (ADR delimiter: "/")
	parts := strings.SplitN(externalName, "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		// Not yet in compound format (e.g. during initial Create before first Observe), skip.
		return nil
	}
	origin := parts[1]

	// ADR(external-name): Writing origin into observation (status.atProvider) rather than parameters
	// ensures it lands in terraform.tfstate via EnsureTFState but not in main.tf.json,
	// which would reject it as a read-only attribute.
	obs, err := cr.GetObservation()
	if err != nil {
		return errors.Wrap(err, "cannot get observation")
	}
	obs["origin"] = origin
	if err := cr.SetObservation(obs); err != nil {
		return errors.Wrap(err, "cannot set observation")
	}
	return errors.Wrap(o.Kube.Status().Update(ctx, cr), "cannot update status")
}
