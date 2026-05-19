package subaccount_trust_configuration

import (
	"context"
	"fmt"

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

		r.References["subaccount_id"] = config.Reference{
			Type:              "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount",
			Extractor:         "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()",
			RefFieldName:      "SubaccountRef",
			SelectorFieldName: "SubaccountSelector",
		}

		// ADR(external-name): returns composite import ID "subaccount_id,origin" required by the TF provider's ImportState function.
		r.ExternalName.GetIDFn = func(_ context.Context, externalName string, parameters map[string]any, _ map[string]any) (string, error) {
			subaccountID, _ := parameters["subaccount_id"].(string)
			if subaccountID == "" || externalName == "" {
				return externalName, nil
			}
			return fmt.Sprintf("%s,%s", subaccountID, externalName), nil
		}

		// ADR(external-name): seeds status.atProvider.origin from the external-name annotation
		// so that terraform refresh can locate the resource without triggering a spurious Create.
		r.InitializerFns = append(r.InitializerFns, func(kube client.Client) managed.Initializer {
			return &OriginInitializer{Kube: kube}
		})
	})
}

// OriginInitializer copies the external-name annotation into status.atProvider.origin
// when the annotation is set but the observation field is still empty.
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

	// ADR(external-name): Writing origin into observation (status.atProvider) rather than parameters
	// ensures it lands in terraform.tfstate via EnsureTFState but not in main.tf.json,
	// which would reject it as a read-only attribute.
	obs, err := cr.GetObservation()
	if err != nil {
		return errors.Wrap(err, "cannot get observation")
	}
	obs["origin"] = externalName
	if err := cr.SetObservation(obs); err != nil {
		return errors.Wrap(err, "cannot set observation")
	}
	return errors.Wrap(o.Kube.Status().Update(ctx, cr), "cannot update status")
}
