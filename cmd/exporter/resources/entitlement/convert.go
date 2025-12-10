package entitlement

import (
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/yaml"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	openapi "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
)

const (
	warnMissingServiceName     = "WARNING: service name is missing in the source, cannot create a Entitlement resource"
	warnMissingServicePlanName = "WARNING: service plan name is missing in the source, cannot create a Entitlement resource"
	warnMissingServicePlanId   = "WARNING: service plan ID is missing in the source, cannot create a valid Entitlement resource"
	warnMissingSubaccountGuid  = "WARNING: subaccount ID is missing in the source, cannot create a valid Entitlement resource"
	warnUndefinedResourceName  = "WARNING: could not generate a valid name for the Entitlement resource"
	warnUnsupportedEntityType  = "WARNING: only 'SUBACCOUNT' entity type is supported for Entitlement resources"
	undefinedName              = "undefined"
)

func convertEntitlementResource(svc *openapi.AssignedServiceResponseObject,
	plan *openapi.AssignedServicePlanResponseObject,
	assignment *openapi.AssignedServicePlanSubaccountDTO) *yaml.ResourceWithComment {

	serviceName, hasServiceName := resources.StringValueOk(svc.GetNameOk())
	servicePlanName, hasPlanName := resources.StringValueOk(plan.GetNameOk())
	subAccountGuid, hasSubaccountGuid := resources.StringValueOk(assignment.GetEntityIdOk())
	entityType, hasEntityType := resources.StringValueOk(assignment.GetEntityTypeOk())

	// TODO: switch to `exporttool/parsan` for name sanitization, once it supports RFC 1123.
	planId, hasPlanId := resources.StringValueOk(plan.GetUniqueIdentifierOk())
	resourceName := resources.SanitizeK8sResourceName(planId+"-"+subAccountGuid, undefinedName)

	// Create Subaccount with required fields first.
	entitlement := yaml.NewResourceWithComment(
		&v1alpha1.Entitlement{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha1.EntitlementKind,
				APIVersion: v1alpha1.CRDGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: resourceName,
				Annotations: map[string]string{
					"crossplane.io/external-name": resourceName,
				},
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

	// Comment the resource out, if any of the required fields is missing.
	if !hasServiceName {
		entitlement.AddComment(warnMissingServiceName)
	}
	if !hasPlanName {
		entitlement.AddComment(warnMissingServicePlanName)
	}
	if !hasPlanId {
		entitlement.AddComment(warnMissingServicePlanId)
	}
	if !hasSubaccountGuid {
		entitlement.AddComment(warnMissingSubaccountGuid)
	}
	if resourceName == undefinedName {
		entitlement.AddComment(warnUndefinedResourceName)
	}
	if !hasEntityType || entityType != "SUBACCOUNT" {
		entitlement.AddComment(warnUnsupportedEntityType + ", but got: '" + entityType + "'")
	}

	return entitlement
}
