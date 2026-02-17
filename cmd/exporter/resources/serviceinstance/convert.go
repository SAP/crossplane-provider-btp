package serviceinstance

import (
	"context"

	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/erratt"
	"github.com/SAP/xp-clifford/yaml"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceinstancebase"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

func convertServiceInstanceResource(ctx context.Context, btpClient *btpcli.BtpCli, si *serviceinstancebase.ServiceInstance, eventHandler export.EventHandler, resolveReferences bool) *yaml.ResourceWithComment {
	serviceName := si.OfferingName
	servicePlanName := si.PlanName
	subAccountGuid := si.SubaccountID
	resourceName := si.GenerateK8sResourceName()
	externalName := si.GetExternalName()
	instanceID := si.ID
	smName := si.ServiceManagerName

	serviceInstance := yaml.NewResourceWithComment(
		&v1alpha1.ServiceInstance{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha1.ServiceInstanceKind,
				APIVersion: v1alpha1.CRDGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: resourceName,
				Annotations: map[string]string{
					"crossplane.io/external-name": externalName,
				},
			},
			Spec: v1alpha1.ServiceInstanceSpec{
				ResourceSpec: v1.ResourceSpec{
					ManagementPolicies: []v1.ManagementAction{
						v1.ManagementActionObserve,
					},
				},
				ForProvider: v1alpha1.ServiceInstanceParameters{
					Name:         si.Name,
					OfferingName: serviceName,
					PlanName:     servicePlanName,
					SubaccountID: &subAccountGuid,
					ServiceManagerRef: &v1.Reference{
						Name: smName,
					},
				},
			},
		})

	// Copy comments from the original resource.
	serviceInstance.CloneComment(si)

	// Comment the resource out, if any of the required fields is missing.
	if serviceName == "" {
		serviceInstance.AddComment(resources.WarnMissingServiceName)
	}
	if servicePlanName == "" {
		serviceInstance.AddComment(resources.WarnMissingServicePlanName)
	}
	if subAccountGuid == "" {
		serviceInstance.AddComment(resources.WarnMissingSubaccountGuid)
	}
	if resourceName == resources.UndefinedName {
		serviceInstance.AddComment(resources.WarnUndefinedResourceName)
	}
	if externalName == "" {
		serviceInstance.AddComment(resources.WarnMissingExternalName)
	}
	if externalName == resources.UndefinedExternalName {
		serviceInstance.AddComment(resources.WarnUndefinedExternalName)
	}
	if instanceID == "" {
		serviceInstance.AddComment(resources.WarnMissingInstanceId)
	}
	if !si.Usable {
		serviceInstance.AddComment(resources.WarnServiceInstanceNotUsable)
	}
	if smName == "" {
		serviceInstance.AddComment(resources.WarnMissingServiceManagerName)
	}

	// Reference subaccount resource, if requested.
	if resolveReferences {
		if err := resolveReference(ctx, btpClient, &serviceInstance.Object.(*v1alpha1.ServiceInstance).Spec.ForProvider); err != nil {
			eventHandler.Warn(erratt.Errorf("cannot resolve subaccount reference: %w", err).With("service instance", si.GetID()))
			serviceInstance.AddComment(resources.WarnCannotResolveSubaccount + ": " + si.SubaccountID)
		}
	}

	return serviceInstance
}

func resolveReference(ctx context.Context, btpClient *btpcli.BtpCli, spec *v1alpha1.ServiceInstanceParameters) error {
	saName, err := subaccount.GetK8sResourceNameByID(ctx, btpClient, *spec.SubaccountID)
	if err != nil {
		return err
	}

	spec.SubaccountRef = &v1.Reference{
		Name: saName,
	}
	spec.SubaccountID = nil

	return nil
}
