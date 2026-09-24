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
	// errServicePlanNotFoundByQualifier reports a four-segment key whose plan
	// name exists but whose unique identifier matches no plan, so the
	// name-only message would point at the wrong field.
	errServicePlanNotFoundByQualifier = "failed to find service plan with the given name %s and unique identifier %s"
)

type EntitlementsClient struct {
	btp btp.Client
}

func NewEntitlementsClient(btp btp.Client) *EntitlementsClient {
	return &EntitlementsClient{btp: btp}

}

// Package-level singleflight + short-TTL cache for GetDirectoryAssignments
// responses, keyed by (subaccountGUID, serviceName, planName); dedupes
// concurrent reconciles and absorbs poll-tick fan-out; writes invalidate
// their key. Fresh reads use a "fresh|"-prefixed key to skip the cache and any in-flight ordinary call.
var (
	describeGroup singleflight.Group
	describeCache sync.Map // string → *describeEntry
)

const describeCacheT = 30 * time.Second

// describeEntry caches one GetDirectoryAssignments response; at records
// when it was issued, for TTL expiry and to reject stale overwrites.
type describeEntry struct {
	val *entclient.EntitledAndAssignedServicesResponseObject
	at  time.Time
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

// describeCacheStore stores val under key with issuedAt as its freshness
// timestamp, unless a newer entry is already cached - this stops a slower
// ordinary/fresh flight from regressing an already-cached response.
func describeCacheStore(key string, val *entclient.EntitledAndAssignedServicesResponseObject, issuedAt time.Time) {
	entry := &describeEntry{val: val, at: issuedAt}
	for {
		actual, loaded := describeCache.LoadOrStore(key, entry)
		if !loaded {
			return
		}
		prev := actual.(*describeEntry)
		if !issuedAt.After(prev.at) {
			return
		}
		if describeCache.CompareAndSwap(key, actual, entry) {
			return
		}
	}
}

func (c EntitlementsClient) DescribeInstance(
	ctx context.Context,
	key ExternalNameKey,
) (*Instance, error) {

	response, err := c.fetchAssignments(ctx, key, false)
	if err != nil {
		return nil, err
	}

	return c.instanceFromResponse(response, key)
}

// DescribeInstanceFresh behaves like DescribeInstance but bypasses the TTL
// cache and any in-flight ordinary call; concurrent fresh calls for the
// same key still coalesce and populate the ordinary cache key on success.
func (c EntitlementsClient) DescribeInstanceFresh(
	ctx context.Context,
	key ExternalNameKey,
) (*Instance, error) {

	response, err := c.fetchAssignments(ctx, key, true)
	if err != nil {
		return nil, err
	}

	return c.instanceFromResponse(response, key)
}

// instanceFromResponse applies the qualifier-aware assigned and entitled
// selectors shared by DescribeInstance and DescribeInstanceFresh to response,
// so both return an *Instance shaped identically for the same payload.
func (c EntitlementsClient) instanceFromResponse(
	response *entclient.EntitledAndAssignedServicesResponseObject,
	key ExternalNameKey,
) (*Instance, error) {
	// assignment can be nil, that is a valid response, as acc/dir will not always have all assignments set
	assignment, err := c.findAssignedServicePlan(response, key)
	if err != nil {
		return nil, err
	}

	entitledServicePlan, errPlan := filterEntitledServices(response, key)

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

// fetchAssignments returns the GetDirectoryAssignments response for key's
// (subaccount, service, plan), reusing a cached value (TTL describeCacheT)
// and deduping concurrent fetches via singleflight; serviceName+planName
// narrows both entitledServices and assignedServices server-side, dropping
// payload ~50-100× vs assignedServiceName alone. fresh routes onto a
// distinct "fresh|"-prefixed flight key to force a real round trip.
func (c EntitlementsClient) fetchAssignments(
	ctx context.Context,
	key ExternalNameKey,
	fresh bool,
) (*entclient.EntitledAndAssignedServicesResponseObject, error) {
	cacheKey := key.CacheKey()
	flightKey := cacheKey
	if fresh {
		flightKey = "fresh|" + cacheKey
	}
	v, err, _ := describeGroup.Do(flightKey, func() (any, error) {
		if !fresh {
			if cached := describeCacheGet(cacheKey); cached != nil {
				return cached, nil
			}
		}
		issuedAt := time.Now()
		resp, _, err := c.btp.EntitlementsServiceClient.
			GetDirectoryAssignments(ctx).
			SubaccountGUID(key.SubaccountGUID).
			ServiceName(key.ServiceName).
			PlanName(key.ServicePlanName).
			Execute()
		if err != nil {
			return nil, err
		}
		describeCacheStore(cacheKey, resp, issuedAt)
		return resp, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*entclient.EntitledAndAssignedServicesResponseObject), nil
}

func (c EntitlementsClient) CreateInstance(ctx context.Context, key ExternalNameKey, cr *v1alpha1.Entitlement) error {
	return c.UpdateInstance(ctx, key, cr)
}

func (c EntitlementsClient) DeleteInstance(ctx context.Context, key ExternalNameKey, cr *v1alpha1.Entitlement) error {
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
	return c.UpdateInstance(ctx, key, cr)
}

func (c EntitlementsClient) UpdateInstance(ctx context.Context, key ExternalNameKey, cr *v1alpha1.Entitlement) error {
	// AutoAssigned entitlements aren't removable by admin action; Create and
	// Delete both funnel through here, so this guard alone covers all three
	// write paths. AutoAssign (user intent) is separate and keeps writing.
	if cr.Status.AtProvider.Assigned != nil && cr.Status.AtProvider.Assigned.AutoAssigned {
		return nil
	}

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
						SubaccountGUID: key.SubaccountGUID,
					},
				},
				ServiceName:                 key.ServiceName,
				ServicePlanName:             key.ServicePlanName,
				ServicePlanUniqueIdentifier: key.ServicePlanUniqueIdentifier,
			},
		},
	)

	_, _, err := c.btp.EntitlementsServiceClient.SetServicePlans(ctx).SubaccountServicePlansRequestPayloadCollection(*payload).Execute()

	if err != nil {
		return specifyAPIError(err, errors.Wrapf(err, errFailedSetEntitlements, key.ServiceName, key.ServicePlanName))
	}

	// Invalidate the singleflight TTL cache so the next Observe reads
	// fresh state instead of pre-write data.
	describeCache.Delete(key.CacheKey())

	return nil
}

// findAssignedServicePlan returns the assignment for the given service and service plan, if it exists
func (c EntitlementsClient) findAssignedServicePlan(payload *entclient.EntitledAndAssignedServicesResponseObject, key ExternalNameKey) (*entclient.AssignedServicePlanSubaccountDTO, error) {
	// first find service via name, can be nil, if no assignment with that service name is set in account/dir
	assignedService := findAssignedService(payload, key.ServiceName)
	if assignedService == nil {
		return nil, nil
	}

	// then find service plan within service, can be nil, if no assignment with that service plan name (and qualifier, if given) is set in account/dir
	servicePlan := findAssignedServicePlanByKey(assignedService, key)
	if servicePlan == nil {
		return nil, nil
	}

	// lastly, extract the info on subaccount entity assignment
	foundAssignment, errLook := filterAssignmentInfo(servicePlan, key)

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

// planMatchesKey reports whether a plan's name and (optional) qualifier
// satisfy key; without a qualifier, key matches on name alone.
func planMatchesKey(name *string, uniqueIdentifier *string, key ExternalNameKey) bool {
	if internal.Val(name) != key.ServicePlanName {
		return false
	}
	return key.ServicePlanUniqueIdentifier == nil || internal.Val(uniqueIdentifier) == *key.ServicePlanUniqueIdentifier
}

// findAssignedServicePlanByKey returns the assigned service plan within
// service whose name and (optional) unique identifier satisfy key, or nil if
// none match.
func findAssignedServicePlanByKey(service *entclient.AssignedServiceResponseObject, key ExternalNameKey) *entclient.AssignedServicePlanResponseObject {
	for i := range service.ServicePlans {
		plan := &service.ServicePlans[i]
		if planMatchesKey(plan.Name, plan.UniqueIdentifier, key) {
			return plan
		}
	}
	return nil
}

// filterAssignmentInfo the api can have multiple assignments for the same service plan, we need to filter by subaccount guid
// (even though having more then one entry here shouldn't be a usecase since we are looking up by subaccount guid)
func filterAssignmentInfo(servicePlan *entclient.AssignedServicePlanResponseObject, key ExternalNameKey) (*entclient.AssignedServicePlanSubaccountDTO, error) {
	var assignment *entclient.AssignedServicePlanSubaccountDTO

	for _, assignmentInfo := range servicePlan.AssignmentInfo {
		if assignmentInfo.EntityId != nil && *assignmentInfo.EntityId == key.SubaccountGUID {
			if assignment != nil {
				return nil, errors.New(errMultipleServicePlans)
			}
			assignment = &assignmentInfo
		}
	}

	return assignment, nil
}

func filterEntitledServices(payload *entclient.EntitledAndAssignedServicesResponseObject, key ExternalNameKey) (*entclient.ServicePlanResponseObject, error) {
	service, err := filterEntitledServiceByName(payload, key.ServiceName)

	if err != nil {
		return nil, err
	}

	servicePlan, errPlan := filterEntitledServicePlan(service, key)

	if errPlan != nil {
		return nil, errPlan
	}

	return servicePlan, nil
}

// filterEntitledServicePlan returns the entitled service plan within service
// whose name and (optional) unique identifier satisfy key.
func filterEntitledServicePlan(
	service *entclient.EntitledServicesResponseObject,
	key ExternalNameKey,
) (*entclient.ServicePlanResponseObject, error) {
	for i := range service.ServicePlans {
		plan := &service.ServicePlans[i]
		if planMatchesKey(plan.Name, plan.UniqueIdentifier, key) {
			return plan, nil
		}
	}
	if key.ServicePlanUniqueIdentifier != nil {
		return nil, errors.Errorf(errServicePlanNotFoundByQualifier,
			key.ServicePlanName, *key.ServicePlanUniqueIdentifier)
	}
	return nil, errors.Errorf(errServicePlanNotFoundByName, key.ServicePlanName)
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
