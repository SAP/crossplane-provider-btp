package directory

import (
	"context"

	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/erratt"
	"github.com/SAP/xp-clifford/yaml"
	v2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

const (
	warnMissingDisplayName = "WARNING: 'displayName' field is missing in the source, cannot set 'DisplayName'"
	warnMissingGuid        = "WARNING: 'guid' field is missing in the source, cannot set 'external-name'"
)

func convertDirectoryResource(ctx context.Context, btpClient *btpcli.BtpCli, d *directory, eventHandler export.EventHandler, resolveReferences bool) *yaml.ResourceWithComment {
	dDisplayName := d.GetDisplayName()
	dGuid := d.GetID()
	resourceName := d.GenerateK8sResourceName()
	externalName := d.GetExternalName()

	// Create Directory with required fields first.
	directoryResource := yaml.NewResourceWithComment(
		&v1alpha1.Directory{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha1.DirectoryKind,
				APIVersion: v1alpha1.CRDGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: resourceName,
				Annotations: map[string]string{
					"crossplane.io/external-name": externalName,
				},
			},
			Spec: v1alpha1.DirectorySpec{
				ClusterManagedResourceSpec: v2.ClusterManagedResourceSpec{
					ManagementPolicies: []v2.ManagementAction{
						v2.ManagementActionObserve,
					},
				},
				ForProvider: v1alpha1.DirectoryParameters{
					DisplayName:       &dDisplayName,
					DirectoryFeatures: d.DirectoryFeatures,
				},
			},
		})

	// Copy comments from the original resource.
	directoryResource.CloneComment(d)

	// Comment the resource out, if any of the required fields is missing.
	if dDisplayName == "" {
		directoryResource.AddComment(warnMissingDisplayName)
	}
	if dGuid == "" {
		directoryResource.AddComment(warnMissingGuid)
	}

	// Fill the optional fields that are relevant for the Update operation, to have it match status.atProvider
	// and not trigger an update right after managementPolicies is set to manage the resource.

	// Description
	if d.Description != "" {
		directoryResource.Resource().(*v1alpha1.Directory).Spec.ForProvider.Description = &d.Description
	}

	// DirectoryGuid: set the parent directory reference only when the parent is
	// another directory, not the global account.
	if d.ParentGUID != "" && d.ParentGUID != d.GlobalAccountGUID {
		directoryResource.Resource().(*v1alpha1.Directory).Spec.ForProvider.DirectoryGuid = d.ParentGUID
	}

	// Labels
	if len(d.Labels) > 0 {
		labels := make(map[string][]string, len(d.Labels))
		for k, v := range d.Labels {
			labels[k] = v
		}
		directoryResource.Resource().(*v1alpha1.Directory).Spec.ForProvider.Labels = labels
	}

	// Reference the parent directory resource, if requested. Only directories
	// whose parent is another (selected) directory carry a DirectoryGuid; those
	// parented by the global account have it empty and are left untouched.
	if resolveReferences {
		if err := resolveReference(ctx, btpClient, &directoryResource.Resource().(*v1alpha1.Directory).Spec.ForProvider); err != nil {
			eventHandler.Warn(erratt.Errorf("cannot resolve directory reference: %w", err).With("directory", d.GetID()))
			directoryResource.AddComment(resources.WarnCannotResolveDirectory + ": " + d.ParentGUID)
		}
	}

	return directoryResource
}

func resolveReference(ctx context.Context, btpClient *btpcli.BtpCli, spec *v1alpha1.DirectoryParameters) error {
	// No parent directory to resolve (parent is the global account).
	if spec.DirectoryGuid == "" {
		return nil
	}

	dName, err := GetK8sResourceNameByID(ctx, btpClient, spec.DirectoryGuid)
	if err != nil {
		return err
	}

	spec.DirectoryRef = &v2.Reference{
		Name: dName,
	}
	spec.DirectoryGuid = ""

	return nil
}
