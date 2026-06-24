package entitlement

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	entclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
	"golang.org/x/sync/singleflight"
)

const (
	errServicePlanNotFound       = "service plan not found"
	errMultipleServicePlans      = "found multiple service plan assignments"
	errFailedSetEntitlements     = "failed to set entitlement for service %s/%s."
	errServiceNotFoundByName     = "failed to find service with the given name %s"
	errServicePlanNotFoundByName = "failed to find service plan with the given name %s"
)

type EntitlementsClient struct {
	btp btp.Client
}

func NewEntitlementsClient(btp btp.Client) *EntitlementsClient {
	return &EntitlementsClient{btp: btp}

}

// ponytail: package-level singleflight + short-TTL cache for
// GetDirectoryAssignments responses. Keyed by (subaccountGUID, serviceName)
// since that's the filter we pass to BTP. Singleflight dedupes concurrent
// sibling reconciles; the TTL absorbs the back-to-back fan-out across all
// CRs hitting the same (subaccount, service) within a single poll tick.
// Writes invalidate their own key so post-write reads see fresh state.
var (
	describeGroup  singleflight.Group
	describeCache  sync.Map // string → *describeEntry
	describeCacheT = 30 * time.Second
)

type describeEntry struct {
	val *entclient.EntitledAndAssignedServicesResponseObject
	at  time.Time
}

func describeKey(subaccountGUID, serviceName string) string {
	return subaccountGUID + "|" + serviceName
}

func describeCacheGet(key string) *entclient.EntitledAndAssignedServicesResponseObject {
	v, ok := describeCache.Load(key)
	if !ok {
		return nil
	}
	e := v.(*describeEntry)
	if time.Since(e.at) > describeCacheT {
		describeCache.Delete(key)
		return nil
	}
	return e.val
}

func describeCachePut(key string, val *entclient.EntitledAndAssignedServicesResponseObject) {
	describeCache.Store(key, &describeEntry{val: val, at: time.Now()})
}

func describeCacheInvalidate(key string) { describeCache.Delete(key) }

func (c EntitlementsClient) DescribeInstance(
	ctx context.Context,
	cr *v1alpha1.Entitlement,
) (*Instance, error) {

	subaccountGUID := cr.Spec.ForProvider.SubaccountGuid
	serviceName := cr.Spec.ForProvider.ServiceName
	planName := cr.Spec.ForProvider.ServicePlanName
	key := describeKey(subaccountGUID, serviceName) + "|" + planName

	response, err := c.fetchAssignments(ctx, key, subaccountGUID, serviceName, planName)
	if err != nil {
		return nil, err
	}

	servicePlanName := planName

	// assignment can be nil, that is a valid response, as acc/dir will anot always have all assignments set
	assignment, err := c.findAssignedServicePlan(response, cr)
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

// fetchAssignments returns the GetDirectoryAssignments response for the given
// (subaccount, service, plan), reusing a cached value (TTL describeCacheT) and
// deduping concurrent fetches via singleflight. serviceName+planName narrows
// BOTH entitledServices and assignedServices server-side, dropping response
// payload ~50-100× vs assignedServiceName alone.
func (c EntitlementsClient) fetchAssignments(ctx context.Context, key, subaccountGUID, serviceName, planName string) (*entclient.EntitledAndAssignedServicesResponseObject, error) {
	if cached := describeCacheGet(key); cached != nil {
		return cached, nil
	}
	v, err, _ := describeGroup.Do(key, func() (any, error) {
		if cached := describeCacheGet(key); cached != nil {
			return cached, nil
		}
		resp, _, err := c.btp.EntitlementsServiceClient.
			GetDirectoryAssignments(ctx).
			SubaccountGUID(subaccountGUID).
			ServiceName(serviceName).
			PlanName(planName).
			Execute()
		if err != nil {
			return nil, err
		}
		describeCachePut(key, resp)
		return resp, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*entclient.EntitledAndAssignedServicesResponseObject), nil
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
		if cr.Status.AtProvider.Required.Amount == nil {
			amount := 0
			cr.Status.AtProvider.Required.Amount = &amount
		}
		// else: Required.Amount already holds the correct sum of remaining CRs
		// (computed by GenerateObservation in Delete() with the deleted CR excluded)
	} else {
		enabled := false
		cr.Status.AtProvider.Required.Enable = &enabled
	}
	return c.UpdateInstance(ctx, cr)
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

	// ponytail: invalidate the singleflight TTL cache so the next Observe
	// reads fresh state instead of pre-write data.
	describeCacheInvalidate(describeKey(cr.Spec.ForProvider.SubaccountGuid, serviceName) + "|" + planName)

	return nil
}

// findAssignedServicePlan returns the assignment for the given service and service plan, if it exists
func (c EntitlementsClient) findAssignedServicePlan(payload *entclient.EntitledAndAssignedServicesResponseObject, cr *v1alpha1.Entitlement) (*entclient.AssignedServicePlanSubaccountDTO, error) {
	// first find service via name, can be nil, if no assignment with that service name is set in account/dir
	assignedService := findAssignedService(payload, cr.Spec.ForProvider.ServiceName)
	if assignedService == nil {
		return nil, nil
	}

	// then find service plan within service, can be nil, if no assignment with that service plan name is set in account/dir
	var servicePlan *entclient.AssignedServicePlanResponseObject
	if cr.Spec.ForProvider.ServicePlanUniqueIdentifier != nil {
		servicePlan = findAssignedServicePlanByNameAndUniqueID(assignedService, cr.Spec.ForProvider.ServicePlanName, *cr.Spec.ForProvider.ServicePlanUniqueIdentifier)
	} else {
		servicePlan = findAssignedServicePlanByName(assignedService, cr.Spec.ForProvider.ServicePlanName)
	}
	if servicePlan == nil {
		return nil, nil
	}

	// lastly, extract the info on subaccount entity assignment
	foundAssignment, errLook := filterAssignmentInfo(servicePlan, cr)

	if errLook != nil {
		return nil, errLook
	}

	return foundAssignment, nil
}

// findAssignedService returns Service if found by name, otherwise nil
func findAssignedService(payload *entclient.EntitledAndAssignedServicesResponseObject, serviceName string) *entclient.AssignedServiceResponseObject {
	for _, assignedService := range payload.AssignedServices {
		if assignedService.Name != nil && *assignedService.Name == serviceName {
			return &assignedService
		}
	}
	return nil
}

// findAssignedServicePlanByName returns servicePlan within service if found by name, otherwise nil
func findAssignedServicePlanByName(service *entclient.AssignedServiceResponseObject, servicePlanName string) *entclient.AssignedServicePlanResponseObject {
	for _, servicePlan := range service.ServicePlans {
		if servicePlan.Name != nil && *servicePlan.Name == servicePlanName {
			return &servicePlan
		}
	}
	return nil
}

// findAssignedServicePlanByNameAndUniqueID returns servicePlan within service if found by name and uniqueID, otherwise nil
func findAssignedServicePlanByNameAndUniqueID(service *entclient.AssignedServiceResponseObject, servicePlanName string, servicePlanUniqueID string) *entclient.AssignedServicePlanResponseObject {
	for _, servicePlan := range service.ServicePlans {
		if servicePlan.Name != nil && *servicePlan.Name == servicePlanName && servicePlan.UniqueIdentifier != nil && *servicePlan.UniqueIdentifier == servicePlanUniqueID {
			return &servicePlan
		}
	}
	return nil
}

// filterAssignmentInfo the api can have multiple assignments for the same service plan, we need to filter by subaccount guid
// (even though having more then one entry here shouldn't be a usecase since we are looking up by subaccount guid)
func filterAssignmentInfo(servicePlan *entclient.AssignedServicePlanResponseObject, cr *v1alpha1.Entitlement) (*entclient.AssignedServicePlanSubaccountDTO, error) {
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

func filterEntitledServiceByName(payload *entclient.EntitledAndAssignedServicesResponseObject, serviceName string) (*entclient.EntitledServicesResponseObject, error) {
	for _, service := range payload.EntitledServices {
		if service.Name != nil && *service.Name == serviceName {
			return &service, nil
		}
	}
	return nil, errors.Errorf(errServiceNotFoundByName, serviceName)
}

// hasNumericQuota checks different factors on the entitlement to understand if it is a numeric one or not - we cannot only deduct that from the service response, since the information we get from the service might be incomplete.
func hasNumericQuota(cr *v1alpha1.Entitlement) bool {
	// use service information, might be incomplete
	if cr.Status.AtProvider.Entitled.Unlimited {
		return false
	}
	return cr.Spec.ForProvider.Amount != nil
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
