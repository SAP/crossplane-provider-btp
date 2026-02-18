package cloudmanagement

import (
	"context"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/erratt"
	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceinstancebase"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

func convertCloudManagementResource(ctx context.Context, btpClient *btpcli.BtpCli, si *serviceinstancebase.ServiceInstance, eventHandler export.EventHandler, resolveReferences bool) *yaml.ResourceWithComment {
	resourceName := si.GenerateK8sResourceName()
	externalName := si.GetExternalName()
	subAccountID := si.SubaccountID
	instanceID := si.ID
	bindingID := si.BindingID
	smName := si.ServiceManagerName

	cm := yaml.NewResourceWithComment(
		&v1beta1.CloudManagement{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1beta1.CloudManagementKind,
				APIVersion: v1beta1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: resourceName,
				Annotations: map[string]string{
					"crossplane.io/external-name": externalName,
				},
			},
			Spec: v1beta1.CloudManagementSpec{
				ResourceSpec: v1.ResourceSpec{
					ManagementPolicies: []v1.ManagementAction{
						v1.ManagementActionObserve,
					},
					WriteConnectionSecretToReference: &v1.SecretReference{
						Name:      resourceName,
						Namespace: resources.DefaultSecretNamespace,
					},
				},
				ForProvider: v1beta1.CloudManagementParameters{
					SubaccountGuid: subAccountID,
					ServiceManagerRef: &v1.Reference{
						Name: smName,
					},
				},
			},
		})

	// Copy comments from the original resource.
	cm.CloneComment(si)

	// Comment the resource out, if any of the required fields is missing.
	if !si.IsCloudManagement() {
		cm.AddComment(resources.WarnNotCloudManagement)
	}
	if subAccountID == "" {
		cm.AddComment(resources.WarnMissingSubaccountGuid)
	}
	if resourceName == resources.UndefinedName {
		cm.AddComment(resources.WarnUndefinedResourceName)
	}
	if externalName == "" {
		cm.AddComment(resources.WarnMissingExternalName)
	}
	if externalName == resources.UndefinedExternalName {
		cm.AddComment(resources.WarnUndefinedExternalName)
	}
	if instanceID == "" {
		cm.AddComment(resources.WarnMissingInstanceId)
	}
	if bindingID == "" {
		cm.AddComment(resources.WarnMissingBindingId)
	}
	if !si.Usable {
		cm.AddComment(resources.WarnServiceInstanceNotUsable)
	}
	if smName == "" {
		cm.AddComment(resources.WarnMissingServiceManagerName)
	}

	// Reference subaccount resource, if requested.
	if resolveReferences {
		if err := resolveReference(ctx, btpClient, &cm.Object.(*v1beta1.CloudManagement).Spec.ForProvider); err != nil {
			eventHandler.Warn(erratt.Errorf("cannot resolve subaccount reference: %w", err).With("cloud management instance", si.GetID()))
			cm.AddComment(resources.WarnCannotResolveSubaccount + ": " + si.SubaccountID)
		}
	}

	return cm
}

func convertDefaultCloudManagementResource(ctx context.Context, btpClient *btpcli.BtpCli, si *serviceinstancebase.ServiceInstance, eventHandler export.EventHandler, resolveReferences bool) *yaml.ResourceWithComment {
	resourceID := si.GetID()
	resourceName := si.GenerateK8sResourceName()
	subAccountID := si.SubaccountID
	smName := si.ServiceManagerName

	cm := yaml.NewResourceWithComment(
		&v1beta1.CloudManagement{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1beta1.CloudManagementKind,
				APIVersion: v1beta1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: resourceName,
			},
			Spec: v1beta1.CloudManagementSpec{
				ResourceSpec: v1.ResourceSpec{
					WriteConnectionSecretToReference: &v1.SecretReference{
						Name:      resourceName,
						Namespace: resources.DefaultSecretNamespace,
					},
				},
				ForProvider: v1beta1.CloudManagementParameters{
					SubaccountGuid: subAccountID,
					ServiceManagerRef: &v1.Reference{
						Name: smName,
					},
				},
			},
		})

	// Copy comments from the original resource.
	cm.CloneComment(si)

	// Comment the resource out, if any of the required fields is missing.
	if resourceID == "" {
		cm.AddComment(resources.WarnMissingInstanceId)
	}
	if subAccountID == "" {
		cm.AddComment(resources.WarnMissingSubaccountGuid)
	}
	if resourceName == resources.UndefinedName {
		cm.AddComment(resources.WarnUndefinedResourceName)
	}
	if smName == "" {
		cm.AddComment(resources.WarnMissingServiceManagerName)
	}

	// Reference subaccount resource, if requested.
	if resolveReferences {
		if err := resolveReference(ctx, btpClient, &cm.Object.(*v1beta1.CloudManagement).Spec.ForProvider); err != nil {
			eventHandler.Warn(erratt.Errorf("cannot resolve subaccount reference: %w", err).With("cloud management instance", si.GetID()))
			cm.AddComment(resources.WarnCannotResolveSubaccount + ": " + si.SubaccountID)
		}
	}

	return cm
}

func resolveReference(ctx context.Context, btpClient *btpcli.BtpCli, spec *v1beta1.CloudManagementParameters) error {
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
