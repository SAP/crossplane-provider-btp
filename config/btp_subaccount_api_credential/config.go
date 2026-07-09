package btp_subaccount_api_credential

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/upjet/v2/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	accountsv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errTrackRUsage                     = "cannot track ResourceUsage"
	errTypeAssertion                   = "managed resource is not of type SubaccountApiCredential"
	errMissingClientSecret             = "cannot read client_secret from source, please delete external resource and re-create Crossplane resource"
	errUpdateExternalName              = "cannot update external-name annotation"
	errMissingNameFromState            = "cannot reconstruct external-name: name missing from tfstate"
	errMissingSubaccountIDFromState    = "cannot reconstruct external-name: subaccount_id missing from tfstate"
	defaultSubaccountApiCredentialName = "managed-subaccount-api-credential"
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

		// ADR(external-name): SubaccountApiCredential is identified by the compound key
		// "<subaccount-id>/<name>". Terraform still expects only the credential name
		// in its "name" argument, so strip the compound-key prefix before rendering
		// Terraform configuration or reconstructed state.
		r.ExternalName.SetIdentifierArgumentFn = setCredentialNameArgument

		r.MetaResource.ArgumentDocs["name"] = "- The name if left unset defaults to managed-subaccount-api-credential"
		if !strings.Contains(r.MetaResource.Description, "Importing or adopting existing API credentials is not supported.") {
			r.MetaResource.Description += ". Importing or adopting existing API credentials is not supported."
		}

		// ADR(external-name): reconstruct the compound key from Terraform state after
		// Create/Observe so Upjet records "<subaccount-id>/<name>" instead of the
		// Terraform resource ID or just the credential name.
		r.ExternalName.GetExternalNameFn = func(tfstate map[string]any) (string, error) {
			return compoundExternalNameFromState(tfstate)
		}

		r.References["subaccount_id"] = config.Reference{
			Type:              "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.Subaccount",
			Extractor:         "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1.SubaccountUuid()",
			RefFieldName:      "SubaccountRef",
			SelectorFieldName: "SubaccountSelector",
		}

		// ADR(external-name): migrate legacy name-only annotations to
		// "<subaccount-id>/<name>" without deleting the existing connection secret.
		// New resources are created without pre-setting external-name; after Create,
		// GetExternalNameFn reconstructs the compound annotation from Terraform state.
		r.InitializerFns = append(r.InitializerFns, func(kube client.Client) managed.Initializer {
			return &CompoundExternalNameInitializer{Kube: kube}
		})
		r.InitializerFns = append(r.InitializerFns, func(kube client.Client) managed.Initializer {
			return &DeletionProtectionInitializer{Kube: kube}
		})
	})

	p.ConfigureResources()
}

func compoundExternalName(subaccountID, credentialName string) string {
	return fmt.Sprintf("%s/%s", subaccountID, credentialName)
}

func splitCompoundExternalName(externalName string) (subaccountID, credentialName string, ok bool) {
	parts := strings.SplitN(externalName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func credentialNameFromExternalName(externalName string) string {
	_, credentialName, ok := splitCompoundExternalName(externalName)
	if ok {
		return credentialName
	}
	return externalName
}

func setCredentialNameArgument(base map[string]any, externalName string) {
	if externalName != "" {
		credentialName := credentialNameFromExternalName(externalName)
		if credentialName != "" {
			base["name"] = credentialName
			return
		}
	}
	if name, ok := base["name"].(string); ok && name != "" {
		return
	}
	base["name"] = defaultSubaccountApiCredentialName
}

func stringField(tfstate map[string]any, field string) string {
	value, _ := tfstate[field].(string)
	return value
}

func compoundExternalNameFromState(tfstate map[string]any) (string, error) {
	subaccountID := stringField(tfstate, "subaccount_id")
	if subaccountID == "" {
		return "", errors.New(errMissingSubaccountIDFromState)
	}
	credentialName := stringField(tfstate, "name")
	if credentialName == "" {
		return "", errors.New(errMissingNameFromState)
	}
	return compoundExternalName(subaccountID, credentialName), nil
}

func specSubaccountID(cr *securityv1alpha1.SubaccountApiCredential) string {
	if cr.Spec.ForProvider.SubaccountID != nil && *cr.Spec.ForProvider.SubaccountID != "" {
		return *cr.Spec.ForProvider.SubaccountID
	}
	return ""
}

func subaccountIDForLegacyMigration(cr *securityv1alpha1.SubaccountApiCredential) string {
	if subaccountID := specSubaccountID(cr); subaccountID != "" {
		return subaccountID
	}
	if cr.Status.AtProvider.SubaccountID != nil && *cr.Status.AtProvider.SubaccountID != "" {
		return *cr.Status.AtProvider.SubaccountID
	}
	return ""
}

// CompoundExternalNameInitializer migrates legacy name-only external-name annotations
// to the ADR-compliant compound key "<subaccount-id>/<name>".
type CompoundExternalNameInitializer struct {
	Kube client.Client
}

func (n *CompoundExternalNameInitializer) Initialize(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*securityv1alpha1.SubaccountApiCredential)
	if !ok {
		return errors.New(errTypeAssertion)
	}

	if meta.WasDeleted(mg) {
		return nil
	}

	currentExternalName := meta.GetExternalName(cr)
	if _, _, ok := splitCompoundExternalName(currentExternalName); ok {
		return nil
	}

	if currentExternalName == "" {
		return nil
	}

	// Legacy annotations used the credential name only. Treat the existing
	// annotation as authoritative during migration to preserve the currently
	// managed external credential, even if spec.forProvider.name was changed.
	// Status is only a fallback for the subaccount ID during legacy migration,
	// because already-managed resources may have observed state while their
	// resolved spec.forProvider.subaccountId is not populated yet.
	subaccountID := subaccountIDForLegacyMigration(cr)
	if subaccountID == "" {
		return nil
	}
	desiredExternalName := compoundExternalName(subaccountID, currentExternalName)

	meta.SetExternalName(cr, desiredExternalName)
	return errors.Wrap(n.Kube.Update(ctx, cr), errUpdateExternalName)
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
	if cr.Spec.ForProvider.SubaccountID != nil && cr.Spec.ForProvider.SubaccountRef != nil {

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

	// According to the Terraform BTP provider docs, client_secret is only generated
	// "if the certificate is omitted". Certificate-based credentials never have a
	// client_secret, so this check must be skipped for them to avoid false positives.
	// See: https://registry.terraform.io/providers/SAP/btp/latest/docs/resources/subaccount_api_credential
	if cr.Spec.ForProvider.CertificatePassed == nil {
		secretRef := cr.GetWriteConnectionSecretToReference()
		if secretRef != nil && secretRef.Name != "" {
			secret := &corev1.Secret{}
			err := d.Kube.Get(ctx, client.ObjectKey{
				Name:      secretRef.Name,
				Namespace: secretRef.Namespace,
			}, secret)
			if err == nil {
				_, hasClientID := secret.Data["attribute.client_id"]
				_, hasTokenURL := secret.Data["attribute.token_url"]
				_, hasAPIURL := secret.Data["attribute.api_url"]
				_, hasClientSecret := secret.Data["attribute.client_secret"]
				if hasClientID && hasTokenURL && hasAPIURL && !hasClientSecret {
					return errors.New(errMissingClientSecret)
				}
			}
		}
	}
	return nil
}
