package cfenvironment

import (
	"context"

	"github.com/SAP/xp-clifford/erratt"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

func convertCloudFoundryEnvResource(ctx context.Context, btpClient *btpcli.BtpCli, e *CloudFoundryEnvironment, eventHandler export.EventHandler, resolveReferences bool) *yaml.ResourceWithComment {
	subAccountGuid := e.SubaccountGUID
	resourceName := e.GenerateK8sResourceName()
	externalName := e.GetExternalName()
	landscape := e.LandscapeLabel
	orgName := e.Labels.OrgName
	envName := e.Name
	cmName := e.CloudManagementName

	cfEnvInstance := yaml.NewResourceWithComment(
		&v1alpha1.CloudFoundryEnvironment{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha1.CfEnvironmentKind,
				APIVersion: v1alpha1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: resourceName,
				Annotations: map[string]string{
					"crossplane.io/external-name": externalName,
				},
			},
			Spec: v1alpha1.CfEnvironmentSpec{
				SubaccountGuid: subAccountGuid,
				ForProvider: v1alpha1.CfEnvironmentParameters{
					Landscape:       landscape,
					OrgName:         orgName,
					EnvironmentName: envName,
				},
				CloudManagementRef: &v1.Reference{
					Name: cmName,
				},
				ResourceSpec: v1.ResourceSpec{
					ManagementPolicies: []v1.ManagementAction{
						v1.ManagementActionObserve,
					},
					WriteConnectionSecretToReference: &v1.SecretReference{
						Name:      resourceName,
						Namespace: resources.DefaultSecretNamespace,
					},
				},
			},
		})

	// Copy comments from the original resource.
	cfEnvInstance.CloneComment(e)

	// Comment the resource out, if any of the important fields is missing.
	if subAccountGuid == "" {
		cfEnvInstance.AddComment(resources.WarnMissingSubaccountGuid)
	}
	if resourceName == resources.UndefinedName {
		cfEnvInstance.AddComment(resources.WarnUndefinedResourceName)
	}
	if externalName == "" {
		cfEnvInstance.AddComment(resources.WarnMissingExternalName)
	}
	if landscape == "" {
		cfEnvInstance.AddComment(resources.WarnMissingLandscapeLabel)
	}
	if orgName == "" {
		cfEnvInstance.AddComment(resources.WarnMissingOrgName)
	}
	if envName == "" {
		cfEnvInstance.AddComment(resources.WarnMissingEnvironmentName)
	}
	if cmName == "" {
		cfEnvInstance.AddComment(resources.WarnMissingCloudManagementName)
	}

	// Reference subaccount resource, if requested.
	if resolveReferences {
		if err := resolveReference(ctx, btpClient, &cfEnvInstance.Object.(*v1alpha1.CloudFoundryEnvironment).Spec); err != nil {
			eventHandler.Warn(erratt.Errorf("cannot resolve subaccount reference: %w", err).With("service instance", e.GetID()))
			cfEnvInstance.AddComment(resources.WarnCannotResolveSubaccount + ": " + e.SubaccountGUID)
		}
	}

	return cfEnvInstance
}

func resolveReference(ctx context.Context, btpClient *btpcli.BtpCli, spec *v1alpha1.CfEnvironmentSpec) error {
	saName, err := subaccount.GetK8sResourceNameByID(ctx, btpClient, spec.SubaccountGuid)
	if err != nil {
		return err
	}

	spec.SubaccountRef = &v1.Reference{
		Name: saName,
	}
	spec.SubaccountGuid = ""

	return nil
}
