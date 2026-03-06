package servicebinding

import (
	"context"

	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/erratt"
	"github.com/SAP/xp-clifford/yaml"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func convertServiceBindingResource(ctx context.Context, btpClient *btpcli.BtpCli, sb *serviceBinding, eventHandler export.EventHandler, resolveReferences bool) resource.Object {
	bindingName := sb.Name
	resourceName := sb.GenerateK8sResourceName()
	externalName := sb.GetExternalName()
	subaccountID := sb.SubaccountID
	instanceID := sb.ServiceInstanceID
	instanceName := sb.ServiceInstanceName

	// Create Entitlement with required fields first.
	managedBinding := yaml.NewResourceWithComment(
		&v1alpha1.ServiceBinding{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha1.ServiceBindingKind,
				APIVersion: v1alpha1.CRDGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: resourceName,
				Annotations: map[string]string{
					"crossplane.io/external-name": externalName,
				},
			},
			Spec: v1alpha1.ServiceBindingSpec{
				ResourceSpec: v1.ResourceSpec{
					ManagementPolicies: []v1.ManagementAction{
						v1.ManagementActionObserve,
					},
				},
				ForProvider: v1alpha1.ServiceBindingParameters{
					Name:              bindingName,
					SubaccountID:      &subaccountID,
					ServiceInstanceID: &instanceID,
					ServiceInstanceRef: &v1.Reference{
						Name: instanceName,
					},
				},
			},
		})

	// Reference subaccount resource, if requested.
	if resolveReferences {
		if err := resolveReference(ctx, btpClient, &managedBinding.Object.(*v1alpha1.ServiceBinding).Spec.ForProvider); err != nil {
			eventHandler.Warn(erratt.Errorf("cannot resolve subaccount reference: %w", err).With("service instance", sb.GetID()))
			managedBinding.AddComment(resources.WarnCannotResolveSubaccount + ": " + subaccountID)
		}
	}

	return managedBinding
}

func resolveReference(ctx context.Context, btpClient *btpcli.BtpCli, spec *v1alpha1.ServiceBindingParameters) error {
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
