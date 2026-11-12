package kymaenvironment

import (
	"context"
	"encoding/json"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/erratt"
	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

func convertKymaEnvResource(ctx context.Context, btpClient *btpcli.BtpCli, e *KymaEnvironment, eventHandler export.EventHandler, resolveReferences bool) *yaml.ResourceWithComment {
	subAccountGuid := e.SubaccountGUID
	resourceName := e.GenerateK8sResourceName()
	externalName := e.GetExternalName()
	name := e.Name
	landscapeLabel := e.LandscapeLabel
	planName := e.PlanName
	cmName := e.CloudManagementName

	forProvider := v1alpha1.KymaEnvironmentParameters{
		PlanName: planName,
		Name:     &name,
	}

	if landscapeLabel != "" {
		forProvider.LandscapeLabel = &landscapeLabel
	}

	if raw, err := json.Marshal(e.Parameters); err == nil {
		forProvider.Parameters = runtime.RawExtension{Raw: raw}
	}

	kymaEnvInstance := yaml.NewResourceWithComment(
		&v1alpha1.KymaEnvironment{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha1.KymaEnvironmentKind,
				APIVersion: v1alpha1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: resourceName,
				Annotations: map[string]string{
					"crossplane.io/external-name": externalName,
				},
			},
			Spec: v1alpha1.KymaEnvironmentSpec{
				SubaccountGuid: subAccountGuid,
				ForProvider:    forProvider,
				CloudManagementRef: &xpv2.Reference{
					Name: cmName,
				},
				ClusterManagedResourceSpec: xpv2.ClusterManagedResourceSpec{
					ManagementPolicies: []xpv2.ManagementAction{
						xpv2.ManagementActionObserve,
					},
					WriteConnectionSecretToReference: &xpv2.SecretReference{
						Name:      resourceName,
						Namespace: resources.DefaultSecretNamespace,
					},
				},
			},
		})

	// Copy comments from the original resource.
	kymaEnvInstance.CloneComment(e)

	// Comment the resource out, if any of the important fields is missing.
	if subAccountGuid == "" {
		kymaEnvInstance.AddComment(resources.WarnMissingSubaccountGuid)
	}
	if resourceName == resources.UndefinedName {
		kymaEnvInstance.AddComment(resources.WarnUndefinedResourceName)
	}
	if externalName == "" {
		kymaEnvInstance.AddComment(resources.WarnMissingExternalName)
	}
	if externalName == resources.UndefinedExternalName {
		kymaEnvInstance.AddComment(resources.WarnUndefinedExternalName)
	}
	if name == "" {
		kymaEnvInstance.AddComment(resources.WarnMissingKymaName)
	}
	if planName == "" {
		kymaEnvInstance.AddComment(resources.WarnMissingKymaPlanName)
	}
	if cmName == "" {
		kymaEnvInstance.AddComment(resources.WarnMissingCloudManagementName)
	}

	// Reference subaccount resource, if requested.
	if resolveReferences {
		if err := resolveReference(ctx, btpClient, &kymaEnvInstance.Object.(*v1alpha1.KymaEnvironment).Spec); err != nil {
			eventHandler.Warn(erratt.Errorf("cannot resolve subaccount reference: %w", err).With("service instance", e.GetID()))
			kymaEnvInstance.AddComment(resources.WarnCannotResolveSubaccount + ": " + e.SubaccountGUID)
		}
	}

	return kymaEnvInstance
}

func resolveReference(ctx context.Context, btpClient *btpcli.BtpCli, spec *v1alpha1.KymaEnvironmentSpec) error {
	saName, err := subaccount.GetK8sResourceNameByID(ctx, btpClient, spec.SubaccountGuid)
	if err != nil {
		return err
	}

	spec.SubaccountRef = &xpv2.Reference{
		Name: saName,
	}
	spec.SubaccountGuid = ""

	return nil
}
