package entitlement

import (
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	entclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
)

const (
	errServicePlanNotFound       = "service plan not found"
	errMultipleServicePlans      = "found multiple service plan assignments"
	errFailedSetEntitlements     = "failed to set entitlement for service %s/%s."
	errServiceNotFoundByName     = "failed to find service with the given name %s"
	errServicePlanNotFoundByName = "failed to find service plan with the given name %s"
	errServiceUniqueName         = "failed to find service plan with the given unique name %s"
	errCannotDetermineOffering   = "Could not determine offering category %s, please open a bug"
)

type EntitlementsClient struct {
	btp btp.Client
}

func NewEntitlementsClient(btp btp.Client) *EntitlementsClient {
	return &EntitlementsClient{btp: btp}

}

func (c EntitlementsClient) DescribeInstance(
	ctx context.Context,
	cr *v1alpha1.Entitlement,
) (*Instance, error) {

	response, _, err := c.btp.EntitlementsServiceClient.
		GetDirectoryAssignments(ctx).
		SubaccountGUID(cr.Spec.ForProvider.SubaccountGuid).
		AssignedServiceName(cr.Spec.ForProvider.ServiceName).
		Execute()
	if err != nil {
		return nil, err
	}

	serviceName := cr.Spec.ForProvider.ServiceName
	servicePlanName := cr.Spec.ForProvider.ServicePlanName

	// assignment can be nil, that is a valid response, as acc/dir will anot always have all assignments set
	assignment, err := filterAssignedServices(response, serviceName, servicePlanName, cr)
	if err != nil {
		return nil, err
	}

	entitledServicePlan, errPlan := filterEntitledServices(response, serviceName, servicePlanName)

	if errPlan != nil {
		return nil, errPlan
	}

	if entitledServicePlan == nil {
		return nil, errors.New(errServicePlanNotFound)
	}

	return &Instance{
		EntitledServicePlan: entitledServicePlan,
		Assignment:          assignment,
	}, nil
}

func filterEntitledServices(payload *entclient.EntitledAndAssignedServicesResponseObject, serviceName string, servicePlanName string) (*entclient.ServicePlanResponseObject, error) {
	service, err := filterEntitledServiceByName(payload, serviceName)

	if err != nil {
		return nil, err
	}

	servicePlan, errPlan := filterEntitledServicePlanByName(service, servicePlanName)

	if errPlan != nil {
		return nil, errPlan
	}

	return servicePlan, nil
}

func filterEntitledServicePlanByName(service *entclient.EntitledServicesResponseObject, servicePlanName string) (*entclient.ServicePlanResponseObject, error) {
	for _, servicePlan := range service.ServicePlans {
		if servicePlan.Name != nil && *servicePlan.Name == servicePlanName {
			return &servicePlan, nil
		}
	}
	return nil, errors.Errorf(errServicePlanNotFoundByName, servicePlanName)
}

func filterAssignedServicePlanByName(service *entclient.AssignedServiceResponseObject, servicePlanName string) (*entclient.AssignedServicePlanResponseObject, error) {
	for _, servicePlan := range service.ServicePlans {
		if servicePlan.Name != nil && *servicePlan.Name == servicePlanName {
			return &servicePlan, nil
		}
	}
	return nil, errors.Errorf(errServicePlanNotFoundByName, servicePlanName)
}

func filterAssignedServicePlanByUniqueID(service *entclient.AssignedServiceResponseObject, servicePlanUniqueID string) error {
	for _, servicePlan := range service.ServicePlans {
		if servicePlan.UniqueIdentifier != nil && *servicePlan.UniqueIdentifier == servicePlanUniqueID {
			return nil
		}
	}
	return errors.Errorf(errServiceUniqueName, servicePlanUniqueID)
}

func filterEntitledServiceByName(payload *entclient.EntitledAndAssignedServicesResponseObject, serviceName string) (*entclient.EntitledServicesResponseObject, error) {
	for _, service := range payload.EntitledServices {
		if service.Name != nil && *service.Name == serviceName {
			return &service, nil
		}
	}
	return nil, errors.Errorf(errServiceNotFoundByName, serviceName)
}

// filterAssignedServiceByName returns *AssignedServiceResponseObject if found, otherwise can also return nil as a valid response
func filterAssignedServiceByName(payload *entclient.EntitledAndAssignedServicesResponseObject, serviceName string) *entclient.AssignedServiceResponseObject {
	for _, assignedService := range payload.AssignedServices {
		if assignedService.Name != nil && *assignedService.Name == serviceName {
			return &assignedService
		}
	}
	return nil
}

func filterAssignedServices(payload *entclient.EntitledAndAssignedServicesResponseObject, serviceName string, servicePlanName string, cr *v1alpha1.Entitlement) (*entclient.AssignedServicePlanSubaccountDTO, error) {
	var assignment *entclient.AssignedServicePlanSubaccountDTO

	// can be nil, if no assignment with that service name is set in account/dir
	assignedService := filterAssignedServiceByName(payload, serviceName)

	if assignedService != nil {
		servicePlan, errPlan := filterAssignedServicePlanByName(assignedService, servicePlanName)

		if errPlan != nil {
			return nil, errPlan
		}

		if cr.Spec.ForProvider.ServicePlanUniqueIdentifier != nil {
			errUnique := filterAssignedServicePlanByUniqueID(assignedService, *cr.Spec.ForProvider.ServicePlanUniqueIdentifier)

			if errUnique != nil {
				return nil, errUnique
			}
		}

		foundAssignment, errLook := lookupAssignmentAndAssign(servicePlan, cr)

		if errLook != nil {
			return nil, errLook
		}

		assignment = foundAssignment
	}

	return assignment, nil
}

func lookupAssignmentAndAssign(servicePlan *entclient.AssignedServicePlanResponseObject, cr *v1alpha1.Entitlement) (*entclient.AssignedServicePlanSubaccountDTO, error) {
	var assignment *entclient.AssignedServicePlanSubaccountDTO

	for _, assignmentInfo := range servicePlan.AssignmentInfo {
		if assignmentInfo.EntityId != nil && *assignmentInfo.EntityId == cr.Spec.ForProvider.SubaccountGuid {
			if assignment != nil {
				return nil, errors.New(errMultipleServicePlans)
			}
			assignment = &assignmentInfo
		}
	}

	return assignment, nil
}

func (c EntitlementsClient) CreateInstance(ctx context.Context, cr *v1alpha1.Entitlement) error {
	return c.UpdateInstance(ctx, cr)
}

func (c EntitlementsClient) DeleteInstance(ctx context.Context, cr *v1alpha1.Entitlement) error {
	// if multiple Entitlements for same plan exist and deleted at the same time, one particular
	// Entitlement might already been cleaned up by the previous run for same plan, then assigned might be nil
	if cr.Status.AtProvider.Assigned == nil {
		return nil
	}

	isNumericQuota := hasNumericQuota(cr)

	// if there is more then one enable entitlement without an amount we can just gracefully remove one
	relatedEntitlements := cr.Status.AtProvider.Required.EntitlementsCount
	if !isNumericQuota && relatedEntitlements != nil && *relatedEntitlements > 1 {
		return nil
	}

	if isNumericQuota {
		amount := 0
		cr.Status.AtProvider.Required.Amount = &amount
	} else {
		enabled := false
		cr.Status.AtProvider.Required.Enable = &enabled
	}
	return c.UpdateInstance(ctx, cr)
}

// hasNumericQuota checks different factors on the entitlement to understand if it is a numeric one or not - we cannot only deduct that from the service response, since the information we get from the service might be incomplete.
func hasNumericQuota(cr *v1alpha1.Entitlement) bool {
	// use service information, might be incomplete
	if cr.Status.AtProvider.Entitled.Unlimited {
		return false
	}
	return cr.Spec.ForProvider.Amount != nil
}

func (c EntitlementsClient) UpdateInstance(ctx context.Context, cr *v1alpha1.Entitlement) error {
	serviceName := cr.Spec.ForProvider.ServiceName
	planName := cr.Spec.ForProvider.ServicePlanName
	servicePlanUniqueIdentifier := cr.Spec.ForProvider.ServicePlanUniqueIdentifier
	var amount *float32
	if cr.Status.AtProvider.Required.Amount != nil {
		amount = internal.Ptr(float32(*cr.Status.AtProvider.Required.Amount))
	}

	payload := entclient.NewSubaccountServicePlansRequestPayloadCollection(
		[]entclient.ServicePlanAssignmentRequestPayload{
			{
				AssignmentInfo: []entclient.SubaccountServicePlanRequestPayload{
					{
						Amount:         amount,
						Enable:         cr.Status.AtProvider.Required.Enable,
						Resources:      nil,
						SubaccountGUID: cr.Spec.ForProvider.SubaccountGuid,
					},
				},
				ServiceName:                 serviceName,
				ServicePlanName:             planName,
				ServicePlanUniqueIdentifier: servicePlanUniqueIdentifier,
			},
		},
	)

	_, _, err := c.btp.EntitlementsServiceClient.SetServicePlans(ctx).SubaccountServicePlansRequestPayloadCollection(*payload).Execute()

	if err != nil {
		return specifyAPIError(err, errors.Wrapf(err, errFailedSetEntitlements, serviceName, planName))
	}

	return nil
}

func float64Pointer(val *int) *float64 {
	if val == nil {
		return nil
	}
	res := float64(*val)
	return &res
}

func isCompleteDeletion(cr *v1alpha1.Entitlement) bool {
	return cr.Status.AtProvider.Required.Amount == nil && cr.Status.AtProvider.Required.Enable == nil
}

func specifyAPIError(err error, fallbackErr error) error {
	if genericErr, ok := err.(*entclient.GenericOpenAPIError); ok {
		if provisionErr, ok := genericErr.Model().(entclient.ApiExceptionResponseObject); ok {
			return errors.New(fmt.Sprintf("API Error: %v, Code %v", internal.Val(provisionErr.Error.Message), internal.Val(provisionErr.Error.Code)))
		}
		if genericErr.Body() != nil {
			return fmt.Errorf("API Error: %s", string(genericErr.Body()))
		}

	}
	return fallbackErr
}
