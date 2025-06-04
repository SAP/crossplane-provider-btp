package btp_subaccount_api_credential

import (
	"context"

	"github.com/pkg/errors"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/upjet/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/client"

	accountsv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"

	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errTrackRUsage   = "cannot track ResourceUsage"
	errTypeAssertion = "managed resource is not of type SubaccountApiCredential"
)

// Configure configures individual resources by adding custom ResourceConfigurators.
func Configure(p *config.Provider) {
	p.AddResourceConfigurator("btp_subaccount_api_credential", func(r *config.Resource) {
		r.ShortGroup = "security"
		r.Kind = "SubaccountApiCredential"
		r.UseAsync = false

		// Mark all as sensitive to exclude them from the status
		r.TerraformResource.Schema["client_secret"].Sensitive = true
		r.TerraformResource.Schema["client_id"].Sensitive = true
		r.TerraformResource.Schema["token_url"].Sensitive = true
		r.TerraformResource.Schema["api_url"].Sensitive = true

		r.ExternalName.SetIdentifierArgumentFn = func(base map[string]any, name string) {
			if name == "" {
				base["name"] = "managed-subbaccount-api-credential"
			} else {
				base["name"] = name
			}
		}

		r.MetaResource.ArgumentDocs["name"] = "The name if left unset defaults to managedsubbaccountapicredential"

		r.ExternalName.GetExternalNameFn = func(tfstate map[string]any) (string, error) {
			return tfstate["name"].(string), nil
		}

		r.References["subaccount_id"] = config.Reference{
			Type:              "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount",
			Extractor:         "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()",
			RefFieldName:      "SubaccountRef",
			SelectorFieldName: "SubaccountSelector",
		}

		// Add pre-delete hook using InitializerFns for finalizer management
		r.InitializerFns = append(r.InitializerFns, func(kube client.Client) managed.Initializer {
			return &DeletionProtectionInitializer{Kube: kube}
		})
	})

	p.ConfigureResources()
}

// DeletionProtectionInitializer implements the managed.Initializer interface
type DeletionProtectionInitializer struct {
	Kube client.Client
}

// Implement the managed.Initializer interface
func (d *DeletionProtectionInitializer) Initialize(ctx context.Context, mg resource.Managed) error {

	// Default reference tracker for tracking references
	referenceTracker := tracking.NewDefaultReferenceResolverTracker(
		d.Kube,
	)

	cr, ok := mg.(*securityv1alpha1.SubaccountApiCredential)

	if !ok {
		return errors.New(errTypeAssertion)
	}

	// Manually define reference tracking for relevant fields
	if cr.Spec.ForProvider.SubaccountID != nil {

		// Use a custom reference tracker to track the subaccount reference
		err := referenceTracker.CreateTrackingReference(ctx, cr, *cr.Spec.ForProvider.SubaccountRef, accountsv1alpha1.SubaccountGroupVersionKind)

		if err != nil {
			return errors.Wrap(err, errTrackRUsage)
		}
	}

	if meta.WasDeleted(mg) {

		referenceTracker.SetConditions(ctx, mg)
		if blocked := referenceTracker.DeleteShouldBeBlocked(mg); blocked {
			return errors.New(providerv1alpha1.ErrResourceInUse)
		}
	}
	return nil
}
