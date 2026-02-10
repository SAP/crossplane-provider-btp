package entitlement

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
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

func convertEntitlementResource(ctx context.Context, btpClient *btpcli.BtpCli, e *entitlement, eventHandler export.EventHandler, resolveReferences bool) *yaml.ResourceWithComment {

	serviceName := e.serviceName
	servicePlanName := e.planName
	subAccountGuid := e.assignment.EntityID
	entityType := e.assignment.EntityType
	resourceName := e.GenerateK8sResourceName()

	// Create Subaccount with required fields first.
	managedEntitlement := yaml.NewResourceWithComment(
		&v1alpha1.Entitlement{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha1.EntitlementKind,
				APIVersion: v1alpha1.CRDGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: resourceName,
			},
			Spec: v1alpha1.EntitlementSpec{
				ResourceSpec: v1.ResourceSpec{
					ManagementPolicies: []v1.ManagementAction{
						v1.ManagementActionObserve,
					},
				},
				ForProvider: v1alpha1.EntitlementParameters{
					ServicePlanName: servicePlanName,
					ServiceName:     serviceName,
					SubaccountGuid:  subAccountGuid,
				},
			},
		})

	// Copy comments from the original resource.
	managedEntitlement.CloneComment(e)

	// Comment the resource out, if any of the required fields is missing.
	if serviceName == "" {
		managedEntitlement.AddComment(resources.WarnMissingServiceName)
	}
	if servicePlanName == "" {
		managedEntitlement.AddComment(resources.WarnMissingServicePlanName)
	}
	if subAccountGuid == "" {
		managedEntitlement.AddComment(resources.WarnMissingSubaccountGuid)
	}
	if resourceName == resources.UndefinedName {
		managedEntitlement.AddComment(resources.WarnUndefinedResourceName)
	}
	if entityType != "SUBACCOUNT" {
		managedEntitlement.AddComment(resources.WarnUnsupportedEntityType + ", but got: '" + entityType + "'")
	}

	// Set optional fields.
	isEnable := e.isEnable()
	if isEnable {
		managedEntitlement.Object.(*v1alpha1.Entitlement).Spec.ForProvider.Enable = &isEnable
	}
	amount := e.assignment.Amount
	if !isEnable && amount >= 1 {
		amountInt := int(amount)
		managedEntitlement.Object.(*v1alpha1.Entitlement).Spec.ForProvider.Amount = &amountInt
	}

	// Reference subaccount resource, if requested.
	if resolveReferences {
		if err := resolveReference(ctx, btpClient, &managedEntitlement.Object.(*v1alpha1.Entitlement).Spec.ForProvider); err != nil {
			eventHandler.Warn(erratt.Errorf("cannot resolve subaccount reference: %w", err).With("entitlement", e.GetID()))
			managedEntitlement.AddComment(resources.WarnCannotResolveSubaccount + ": " + subAccountGuid)
		}
	}

	return managedEntitlement
}

func resolveReference(ctx context.Context, btpClient *btpcli.BtpCli, entitlement *v1alpha1.EntitlementParameters) error {
	saName, err := subaccount.GetK8sResourceNameByID(ctx, btpClient, entitlement.SubaccountGuid)
	if err != nil {
		return err
	}

	entitlement.SubaccountRef = &v1.Reference{
		Name: saName,
	}
	entitlement.SubaccountGuid = ""

	return nil
}
