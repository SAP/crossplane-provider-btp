package servicemanager

import (
	"context"

	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/erratt"
	"github.com/SAP/xp-clifford/yaml"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

func convertServiceManagerResource(ctx context.Context, btpClient *btpcli.BtpCli, si *ServiceInstance, eventHandler export.EventHandler, resolveReferences bool) *yaml.ResourceWithComment {
	resourceName := si.GenerateK8sResourceName()
	externalName := si.GetExternalName()
	subAccountID := si.SubaccountID
	instanceID := si.ID
	bindingID := si.bindingID

	serviceManager := yaml.NewResourceWithComment(
		&v1beta1.ServiceManager{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1beta1.ServiceManagerKind,
				APIVersion: v1beta1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: resourceName,
				Annotations: map[string]string{
					"crossplane.io/external-name": externalName,
				},
			},
			Spec: v1beta1.ServiceManagerSpec{
				ResourceSpec: v1.ResourceSpec{
					ManagementPolicies: []v1.ManagementAction{
						v1.ManagementActionObserve,
					},
					WriteConnectionSecretToReference: &v1.SecretReference{
						Name:      resourceName,
						Namespace: DefaultSecretNamespace,
					},
				},
				ForProvider: v1beta1.ServiceManagerParameters{
					SubaccountGuid: subAccountID,
				},
			},
		})

	// Copy comments from the original resource.
	serviceManager.CloneComment(si)

	// Comment the resource out, if any of the required fields is missing.
	if subAccountID == "" {
		serviceManager.AddComment(resources.WarnMissingSubaccountGuid)
	}
	if resourceName == resources.UndefinedName {
		serviceManager.AddComment(resources.WarnUndefinedResourceName)
	}
	if externalName == "" {
		serviceManager.AddComment(resources.WarnMissingExternalName)
	}
	if instanceID == "" {
		serviceManager.AddComment(resources.WarnMissingInstanceId)
	}
	if bindingID == "" {
		serviceManager.AddComment(resources.WarnMissingBindingId)
	}
	if !si.Usable {
		serviceManager.AddComment(resources.WarnServiceInstanceNotUsable)
	}

	// Reference subaccount resource, if requested.
	if resolveReferences {
		if err := resolveReference(ctx, btpClient, &serviceManager.Object.(*v1beta1.ServiceManager).Spec.ForProvider); err != nil {
			eventHandler.Warn(erratt.Errorf("cannot resolve subaccount reference: %w", err).With("service instance", si.GetID()))
			serviceManager.AddComment(resources.WarnCannotResolveSubaccount + ": " + si.SubaccountID)
		}
	}

	return serviceManager
}

func defaultServiceManagerResource(ctx context.Context, btpClient *btpcli.BtpCli, subaccountID string, eventHandler export.EventHandler, resolveReferences bool) *yaml.ResourceWithComment {
	resourceName := defaultServiceManagerResourceName(subaccountID)

	serviceManager := yaml.NewResourceWithComment(
		&v1beta1.ServiceManager{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1beta1.ServiceManagerKind,
				APIVersion: v1beta1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: resourceName,
			},
			Spec: v1beta1.ServiceManagerSpec{
				ResourceSpec: v1.ResourceSpec{
					WriteConnectionSecretToReference: &v1.SecretReference{
						Name:      resourceName,
						Namespace: DefaultSecretNamespace,
					},
				},
				ForProvider: v1beta1.ServiceManagerParameters{
					SubaccountGuid: subaccountID,
				},
			},
		})

	// Comment the resource out, if any of the required fields is missing.
	if subaccountID == "" {
		serviceManager.AddComment(resources.WarnMissingSubaccountGuid)
	}

	// Reference subaccount resource, if requested.
	if resolveReferences {
		if err := resolveReference(ctx, btpClient, &serviceManager.Object.(*v1beta1.ServiceManager).Spec.ForProvider); err != nil {
			eventHandler.Warn(erratt.Errorf("cannot resolve subaccount reference: %w", err).With("subaccount", subaccountID))
			serviceManager.AddComment(resources.WarnCannotResolveSubaccount + ": " + subaccountID)
		}
	}

	return serviceManager
}

func resolveReference(ctx context.Context, btpClient *btpcli.BtpCli, spec *v1beta1.ServiceManagerParameters) error {
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
