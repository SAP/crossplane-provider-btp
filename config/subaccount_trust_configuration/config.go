package subaccount_trust_configuration

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/upjet/pkg/config"
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

		applyDescriptionInconsistencyWorkaround(r)

		r.References["subaccount_id"] = config.Reference{
			Type:              "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount",
			Extractor:         "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()",
			RefFieldName:      "SubaccountRef",
			SelectorFieldName: "SubaccountSelector",
		}

	})
}

// applyDescriptionInconsistencyWorkaround works around an upstream bug in
// terraform-provider-btp (see #724, fixed upstream in v1.23.1) that causes
// SubaccountTrustConfiguration creation to fail with "Provider produced
// inconsistent result after apply" when spec.forProvider.description is set.
//
// Registers an Initializer that nils spec.forProvider.description (and the
// initProvider counterpart) in-process on every reconcile, before upjet
// serializes the spec into main.tf.json. Terraform therefore plans against a
// nil description; combined with Optional+Computed in the schema, the
// BTP-computed value is accepted without a consistency violation. The change
// is not persisted to the API server, so the user's spec value remains intact
// and the workaround is transparent on removal. Also overrides the field's
// ArgumentDocs to surface the limitation on the user-facing CR field.
//
// TODO: remove when terraform-provider-btp is bumped to >= 1.23.1 (#521).
func applyDescriptionInconsistencyWorkaround(r *config.Resource) {
	r.InitializerFns = append(r.InitializerFns, func(_ client.Client) managed.Initializer {
		return &descriptionStripper{}
	})
	if r.MetaResource != nil && r.MetaResource.ArgumentDocs != nil {
		r.MetaResource.ArgumentDocs["description"] = "(String) Description of the trust configuration. " +
			"NOTE: currently ignored due to an upstream bug (see #724); the BTP backend assigns its own default."
	}
}

// descriptionStripper is the Initializer registered by
// applyDescriptionInconsistencyWorkaround. See that function's godoc for
// rationale and removal conditions.
type descriptionStripper struct{}

func (descriptionStripper) Initialize(_ context.Context, mg resource.Managed) error {
	cr, ok := mg.(*securityv1alpha1.SubaccountTrustConfiguration)
	if !ok {
		return errors.New(errTypeAssertion)
	}
	cr.Spec.ForProvider.Description = nil
	cr.Spec.InitProvider.Description = nil
	return nil
}
