package globalaccount_trust_configuration

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/upjet/v2/pkg/config"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

const errTypeAssertion = "managed resource is not of type GlobalaccountTrustConfiguration"

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_globalaccount_trust_configuration", func(r *config.Resource) {
		r.ShortGroup = "security"
		r.Kind = "GlobalaccountTrustConfiguration"

		// The provider's Read keys off state.origin, not id, so set origin from the external-name.
		// Without it import/observe reads an empty origin -> 404 -> Create -> 409 "already exists".
		r.ExternalName.SetIdentifierArgumentFn = func(base map[string]any, externalName string) {
			base["origin"] = externalName
		}

		// Sets status.atProvider.origin from the external-name so a refresh with existing TF state
		// (but origin still unset) locates the resource instead of triggering an unwanted Create.
		r.InitializerFns = append(r.InitializerFns, func(kube client.Client) managed.Initializer {
			return &OriginInitializer{Kube: kube}
		})
	})
}

// OriginInitializer copies the external-name annotation (the origin key) into
// status.atProvider.origin when the annotation is set but the observation field is still empty.
type OriginInitializer struct {
	Kube client.Client
}

func (o *OriginInitializer) Initialize(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*securityv1alpha1.GlobalaccountTrustConfiguration)
	if !ok {
		return errors.New(errTypeAssertion)
	}

	externalName := meta.GetExternalName(cr)
	if externalName == "" || cr.Status.AtProvider.Origin != nil {
		return nil
	}

	// Writing origin into observation (status.atProvider) rather than parameters puts it in
	// terraform.tfstate via EnsureTFState but not in main.tf.json, which would reject it as read-only.
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
