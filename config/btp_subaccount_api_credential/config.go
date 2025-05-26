package btp_subaccount_api_credential

import (
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/upjet/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/client"

	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

const (
	// Custom finalizer to prevent deletion when resource is in use
	finalizerDeletionProtection = "security.btp.crossplane.io/deletion-protection"
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
			return managed.NewNameAsExternalName(kube)
		})
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
	return d.InitializeExternal(ctx, mg, nil)
}

// InitializeExternal handles the custom finalizer logic
func (d *DeletionProtectionInitializer) InitializeExternal(ctx context.Context, mg resource.Managed, tc managed.ExternalClient) error {
	cr, ok := mg.(*securityv1alpha1.SubaccountApiCredential)
	if !ok {
		return fmt.Errorf("managed resource is not a SubaccountApiCredential")
	}

	// Add our custom finalizer if it doesn't exist and resource is not being deleted
	if cr.GetDeletionTimestamp() == nil {
		finalizers := cr.GetFinalizers()
		hasFinalizer := false
		for _, f := range finalizers {
			if f == finalizerDeletionProtection {
				hasFinalizer = true
				break
			}
		}
		if !hasFinalizer {
			finalizers = append(finalizers, finalizerDeletionProtection)
			cr.SetFinalizers(finalizers)
		}
		return nil
	}

	// If resource is being deleted, check if it's safe to proceed
	if cr.GetDeletionTimestamp() != nil {
		// Check if the resource is still in use
		inUse, err := d.isResourceInUse(ctx, cr, tc)
		if err != nil {
			return fmt.Errorf("failed to check if resource is in use: %w", err)
		}

		if inUse {
			// Resource is still in use, don't proceed with deletion
			return fmt.Errorf("SubaccountApiCredential %s cannot be deleted as it is still being referenced by other resources", cr.GetName())
		}

		// Resource is not in use, remove our custom finalizer to allow deletion
		finalizers := cr.GetFinalizers()
		newFinalizers := make([]string, 0, len(finalizers))
		for _, f := range finalizers {
			if f != finalizerDeletionProtection {
				newFinalizers = append(newFinalizers, f)
			}
		}
		cr.SetFinalizers(newFinalizers)
	}

	return nil
}

// isResourceInUse checks if the SubaccountApiCredential is referenced by other resources
func (d *DeletionProtectionInitializer) isResourceInUse(ctx context.Context, cr *securityv1alpha1.SubaccountApiCredential, _ managed.ExternalClient) (bool, error) {
	k8sClient := d.Kube
	if k8sClient == nil {
		return true, fmt.Errorf("unable to get Kubernetes client for reference checking")
	}

	// Check for force-delete annotation that bypasses protection
	if annotations := cr.GetAnnotations(); annotations != nil {
		if forceDelete, exists := annotations["security.btp.crossplane.io/force-delete"]; exists && forceDelete == "true" {
			return false, nil
		}
	}

	// Check if any RoleCollection resources reference this SubaccountApiCredential
	roleCollections := &securityv1alpha1.RoleCollectionList{}
	if err := k8sClient.List(ctx, roleCollections); err != nil {
		return false, fmt.Errorf("failed to list RoleCollections: %w", err)
	}

	credName := cr.GetName()
	credNamespace := cr.GetNamespace()

	for _, rc := range roleCollections.Items {
		if d.roleCollectionReferencesCredential(&rc, credName, credNamespace) {
			return true, fmt.Errorf("RoleCollection %s/%s still references this SubaccountApiCredential", rc.GetNamespace(), rc.GetName())
		}
	}

	// Check if any RoleCollectionAssignment resources reference this SubaccountApiCredential
	roleCollectionAssignments := &securityv1alpha1.RoleCollectionAssignmentList{}
	if err := k8sClient.List(ctx, roleCollectionAssignments); err != nil {
		return false, fmt.Errorf("failed to list RoleCollectionAssignments: %w", err)
	}

	for _, rca := range roleCollectionAssignments.Items {
		if d.roleCollectionAssignmentReferencesCredential(&rca, credName, credNamespace) {
			return true, fmt.Errorf("RoleCollectionAssignment %s/%s still references this SubaccountApiCredential", rca.GetNamespace(), rca.GetName())
		}
	}

	// No references found, safe to delete
	return false, nil
}

// roleCollectionReferencesCredential checks if a RoleCollection references the SubaccountApiCredential
func (d *DeletionProtectionInitializer) roleCollectionReferencesCredential(rc *securityv1alpha1.RoleCollection, credName, credNamespace string) bool {
	ref := rc.Spec.XSUAACredentialsReference.SubaccountApiCredentialRef
	selector := rc.Spec.XSUAACredentialsReference.SubaccountApiCredentialSelector
	if ref != nil {
		if ref.Name == credName {
			// xpv1.Reference does not have Namespace, so assume same namespace as the referencing resource
			return rc.GetNamespace() == credNamespace
		}
	}
	if selector != nil {
		// If using selector, conservatively assume it might reference our credential
		return true
	}
	return false
}

// roleCollectionAssignmentReferencesCredential checks if a RoleCollectionAssignment references the SubaccountApiCredential
func (d *DeletionProtectionInitializer) roleCollectionAssignmentReferencesCredential(rca *securityv1alpha1.RoleCollectionAssignment, credName, credNamespace string) bool {
	ref := rca.Spec.XSUAACredentialsReference.SubaccountApiCredentialRef
	selector := rca.Spec.XSUAACredentialsReference.SubaccountApiCredentialSelector
	if ref != nil {
		if ref.Name == credName {
			// xpv1.Reference does not have Namespace, so assume same namespace as the referencing resource
			return rca.GetNamespace() == credNamespace
		}
	}
	if selector != nil {
		// If using selector, conservatively assume it might reference our credential
		return true
	}
	return false
}
