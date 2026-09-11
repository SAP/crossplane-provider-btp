package serviceinstanceclient

import (
	"context"
	"net/http"
	"path"
	"sort"
	"sync"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	smopenapi "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
)

// Label operations for the Service Manager PATCH label diff.
const (
	labelOpAdd    = "add"
	labelOpRemove = "remove"
)

// reservedLabels are labels the Service Manager attaches and manages itself.
// They are returned on the instance GET but are not user-settable: attempting
// to add/remove them yields "Modifying is not allowed for label <key>".
// They must be excluded from both the drift comparison and the label PATCH diff.
var reservedLabels = map[string]struct{}{
	"subaccount_id": {},
}

// isReservedLabel reports whether a label key is Service-Manager-managed and
// must not be included in user-driven label operations or drift detection.
func isReservedLabel(key string) bool {
	_, ok := reservedLabels[key]
	return ok
}

// FilterReservedLabels returns a copy of the label map with SM-managed reserved
// keys removed. Exported so the controller can filter observed labels before
// drift comparison. Returns nil for nil input.
func FilterReservedLabels(in map[string][]*string) map[string][]*string {
	if in == nil {
		return nil
	}
	out := make(map[string][]*string, len(in))
	for k, v := range in {
		if isReservedLabel(k) {
			continue
		}
		out[k] = v
	}
	return out
}

// ServiceInstanceClientI abstracts the native Service Manager operations used
// by the ServiceInstance controller.
type ServiceInstanceClientI interface {
	Observe(ctx context.Context, externalName string) (ObserveResult, error)
	// Create provisions a new service instance and returns (id, operationID, error).
	// operationID is the async SM operation id (may be empty for sync creates).
	Create(ctx context.Context, cr *v1alpha1.ServiceInstance, params map[string]interface{}) (id string, operationID string, err error)
	// Update applies at most one PATCH per call. It returns the SM operation id
	// for the general async PATCH, or "" for the shared-only sync PATCH.
	Update(ctx context.Context, externalName string, cr *v1alpha1.ServiceInstance,
		params map[string]interface{}, observed *smopenapi.ServiceInstanceResponseObject) (operationID string, err error)
	Delete(ctx context.Context, externalName string) error
	// GetParameters fetches the current parameters of a service instance from SM.
	GetParameters(ctx context.Context, instanceID string) (map[string]interface{}, error)
	// InstancesRetrievable resolves whether the service offering associated with
	// the given service plan id supports the /parameters endpoint.
	InstancesRetrievable(ctx context.Context, planID string) (bool, error)
}

// ObserveResult is the outcome of an Observe call against the Service Manager API.
type ObserveResult struct {
	Exists   bool
	Instance *smopenapi.ServiceInstanceResponseObject // nil when !Exists
}

// ServiceInstanceClient wraps each Service Manager call behind a function field
// (defaulting to the real SM call) so table-driven tests can substitute mocks
// without fighting the generated request-builder chain.
type ServiceInstanceClient struct {
	createFn       func(ctx context.Context, payload smopenapi.CreateServiceInstanceRequestPayload, async bool) (*smopenapi.CreatedServiceInstanceResponseObject, *http.Response, error)
	getFn          func(ctx context.Context, id string) (*smopenapi.ServiceInstanceResponseObject, *http.Response, error)
	updateFn       func(ctx context.Context, id string, payload smopenapi.UpdateServiceInstanceRequestPayload, async bool) (*smopenapi.UpdatedServiceInstanceResponseObject, *http.Response, error)
	deleteFn       func(ctx context.Context, id string, async bool) (map[string]interface{}, *http.Response, error)
	getParamsFn    func(ctx context.Context, id string) (map[string]interface{}, *http.Response, error)
	getPlanFn      func(ctx context.Context, planID string) (*smopenapi.ServicePlanResponseObject, *http.Response, error)
	getOfferingFn  func(ctx context.Context, offeringID string) (*smopenapi.ServiceOfferingResponseObject, *http.Response, error)

	// retrievableCache maps offering id -> instances_retrievable, populated on
	// first lookup. Safe for concurrent read/write via mu.
	mu              sync.RWMutex
	retrievableCache map[string]bool
}

var _ ServiceInstanceClientI = &ServiceInstanceClient{}

// NewServiceInstanceClient builds a native client from a ServiceManagerClient
// (which embeds the generated ServiceInstancesAPI). Each function field is
// wired to the real SM request-builder chain.
func NewServiceInstanceClient(smc *smClient.ServiceManagerClient) *ServiceInstanceClient {
	return &ServiceInstanceClient{
		createFn: func(ctx context.Context, payload smopenapi.CreateServiceInstanceRequestPayload, async bool) (*smopenapi.CreatedServiceInstanceResponseObject, *http.Response, error) {
			return smc.CreateServiceInstance(ctx).CreateServiceInstanceRequestPayload(payload).Async(async).Execute()
		},
		getFn: func(ctx context.Context, id string) (*smopenapi.ServiceInstanceResponseObject, *http.Response, error) {
			return smc.GetServiceInstanceById(ctx, id).Execute()
		},
		updateFn: func(ctx context.Context, id string, payload smopenapi.UpdateServiceInstanceRequestPayload, async bool) (*smopenapi.UpdatedServiceInstanceResponseObject, *http.Response, error) {
			return smc.UpdateServiceInstance(ctx, id).UpdateServiceInstanceRequestPayload(payload).Async(async).Execute()
		},
		deleteFn: func(ctx context.Context, id string, async bool) (map[string]interface{}, *http.Response, error) {
			return smc.DeleteServiceInstance(ctx, id).Async(async).Execute()
		},
		getParamsFn: func(ctx context.Context, id string) (map[string]interface{}, *http.Response, error) {
			return smc.GetServiceInstanceParameters(ctx, id).Execute()
		},
		getPlanFn: func(ctx context.Context, planID string) (*smopenapi.ServicePlanResponseObject, *http.Response, error) {
			return smc.GetServicePlansByServiceId(ctx, planID).Execute()
		},
		getOfferingFn: func(ctx context.Context, offeringID string) (*smopenapi.ServiceOfferingResponseObject, *http.Response, error) {
			return smc.GetServiceOfferingById(ctx, offeringID).Execute()
		},
		retrievableCache: make(map[string]bool),
	}
}

// Observe fetches a service instance by ID. A 404 maps to {Exists:false, nil}
// so the controller can trigger create/recovery.
func (c *ServiceInstanceClient) Observe(ctx context.Context, externalName string) (ObserveResult, error) {
	resp, httpRes, err := c.getFn(ctx, externalName)
	if err != nil {
		if isNotFound(httpRes) {
			return ObserveResult{Exists: false}, nil
		}
		return ObserveResult{}, smClient.SpecifyAPIError(err)
	}
	return ObserveResult{Exists: true, Instance: resp}, nil
}

// Create provisions a service instance by plan ID. `shared` is deliberately NOT
// sent at create (the create payloads have no such field); it is reconciled by a
// later Update once the instance is Ready. Returns the new instance GUID and the
// SM async operation id (from the Location header of the 202 response).
func (c *ServiceInstanceClient) Create(ctx context.Context, cr *v1alpha1.ServiceInstance, params map[string]interface{}) (string, string, error) {
	byPlan := smopenapi.NewCreateByPlanID(cr.Spec.ForProvider.Name, cr.Status.AtProvider.ServiceplanID)
	if len(params) > 0 {
		byPlan.SetParameters(params)
	}
	if labels := toStringSliceMap(cr.Spec.ForProvider.Labels); len(labels) > 0 {
		byPlan.SetLabels(labels)
	}
	payload := smopenapi.CreateByPlanIDAsCreateServiceInstanceRequestPayload(byPlan)

	resp, httpRes, err := c.createFn(ctx, payload, true)
	if err != nil {
		return "", "", smClient.SpecifyAPIError(err)
	}
	if resp == nil {
		return "", "", errors.New("service manager returned no service instance on create")
	}
	return resp.GetId(), operationIDFromResponse(httpRes), nil
}

// Update applies at most ONE PATCH per call. The Service Manager `shared`
// property is special: it must be sent ALONE in the request body and only
// synchronously ("Async requests are not supported when modifying the shared
// property"; "you must pass the shareable boolean alone"). It also cannot be
// combined with a concurrent async operation on the same instance. So when
// shared drifts we issue only the synchronous shared-only PATCH and let the
// controller's next reconcile handle any remaining (general) drift. When shared
// does not drift, we issue the general async PATCH (name, plan, params, labels).
//
// Returns the SM operation id from the async PATCH's Location header, or "" for
// the shared-only sync PATCH (which carries no parameters).
func (c *ServiceInstanceClient) Update(ctx context.Context, externalName string, cr *v1alpha1.ServiceInstance,
	params map[string]interface{}, observed *smopenapi.ServiceInstanceResponseObject) (string, error) {
	// Shared drift takes priority and is applied on its own, synchronously.
	// The controller requeues afterwards, so remaining drift is reconciled next loop.
	if sharedNeedsUpdate(cr, observed) {
		sharedPayload := smopenapi.NewUpdateServiceInstanceRequestPayload()
		sharedPayload.Shared = cr.Spec.ForProvider.Shared
		if _, _, err := c.updateFn(ctx, externalName, *sharedPayload, false); err != nil {
			return "", smClient.SpecifyAPIError(err)
		}
		// shared-only sync PATCH: no operation id to track.
		return "", nil
	}

	payload := smopenapi.NewUpdateServiceInstanceRequestPayload()
	payload.Name = internal.Ptr(cr.Spec.ForProvider.Name)
	if cr.Status.AtProvider.ServiceplanID != "" {
		payload.ServicePlanId = internal.Ptr(cr.Status.AtProvider.ServiceplanID)
	}
	if len(params) > 0 {
		payload.Parameters = params
	}
	if labels := diffLabels(cr.Spec.ForProvider.Labels, observedLabels(observed)); len(labels) > 0 {
		payload.Labels = labels
	}

	_, httpRes, err := c.updateFn(ctx, externalName, *payload, true)
	if err != nil {
		return "", smClient.SpecifyAPIError(err)
	}
	return operationIDFromResponse(httpRes), nil
}

// sharedNeedsUpdate reports whether the managed `shared` value differs from the
// observed instance and therefore requires its own synchronous PATCH. Returns
// false when shared is unmanaged (spec.Shared == nil).
func sharedNeedsUpdate(cr *v1alpha1.ServiceInstance, observed *smopenapi.ServiceInstanceResponseObject) bool {
	if cr.Spec.ForProvider.Shared == nil {
		return false
	}
	var observedShared bool
	if observed != nil {
		observedShared = observed.GetShared()
	}
	return internal.Val(cr.Spec.ForProvider.Shared) != observedShared
}

// Delete initiates async deletion. A 404 is treated as success (already gone).
func (c *ServiceInstanceClient) Delete(ctx context.Context, externalName string) error {
	_, httpRes, err := c.deleteFn(ctx, externalName, true)
	if err != nil {
		if isNotFound(httpRes) {
			return nil
		}
		return smClient.SpecifyAPIError(err)
	}
	return nil
}

// GetParameters fetches the current parameters of a service instance from SM.
// A non-2xx response is returned as an error via smClient.SpecifyAPIError.
func (c *ServiceInstanceClient) GetParameters(ctx context.Context, instanceID string) (map[string]interface{}, error) {
	params, _, err := c.getParamsFn(ctx, instanceID)
	if err != nil {
		return nil, smClient.SpecifyAPIError(err)
	}
	return params, nil
}

// InstancesRetrievable resolves whether the service offering for the given plan
// id exposes /parameters (instances_retrievable == true). The result is cached
// per offering id (it is stable — an offering's retrievable flag never changes).
func (c *ServiceInstanceClient) InstancesRetrievable(ctx context.Context, planID string) (bool, error) {
	// 1. Fetch the plan to get its offering id.
	plan, _, err := c.getPlanFn(ctx, planID)
	if err != nil {
		return false, smClient.SpecifyAPIError(err)
	}
	offeringID := plan.GetServiceOfferingId()

	// 2. Check the cache.
	c.mu.RLock()
	if v, ok := c.retrievableCache[offeringID]; ok {
		c.mu.RUnlock()
		return v, nil
	}
	c.mu.RUnlock()

	// 3. Fetch the offering.
	offering, _, err := c.getOfferingFn(ctx, offeringID)
	if err != nil {
		return false, smClient.SpecifyAPIError(err)
	}
	retrievable := offering.GetInstancesRetrievable()

	// 4. Store in cache.
	c.mu.Lock()
	c.retrievableCache[offeringID] = retrievable
	c.mu.Unlock()

	return retrievable, nil
}

// operationIDFromResponse extracts the SM async operation id from the Location
// header of a 202 response. The header value is a path of the form
// /v1/service_instances/{id}/operations/{opId}; we return the last path segment.
// Returns "" when the header is absent or the response is nil.
func operationIDFromResponse(res *http.Response) string {
	if res == nil {
		return ""
	}
	loc := res.Header.Get("Location")
	if loc == "" {
		return ""
	}
	return path.Base(loc)
}


// with the spec parameters, returning the combined map. It is the map-returning
// sibling of BuildComplexParameterJson (which is kept untouched for the
// servicebinding consumer).
func BuildComplexParameterMap(ctx context.Context, kube client.Client, secretRefs []xpv1.SecretKeySelector, specParams []byte) (map[string]interface{}, error) {
	parameterData, err := lookupSecrets(ctx, kube, secretRefs)
	if err != nil {
		return nil, err
	}

	specParamsMap, err := internal.UnmarshalRawParameters(specParams)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal spec parameters")
	}
	addMap(parameterData, specParamsMap)
	return parameterData, nil
}

// isNotFound reports whether the HTTP response is a 404, detected by status code
// (not string matching).
func isNotFound(res *http.Response) bool {
	return res != nil && res.StatusCode == http.StatusNotFound
}

// toStringSliceMap dereferences a map[string][]*string into map[string][]string,
// skipping nil entries.
func toStringSliceMap(in map[string][]*string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for k, vals := range in {
		strs := make([]string, 0, len(vals))
		for _, v := range vals {
			if v == nil {
				continue
			}
			strs = append(strs, *v)
		}
		out[k] = strs
	}
	return out
}

// ToPtrSliceMap converts a map[string][]string into map[string][]*string, the
// shape used by ServiceInstance spec labels. Exported for reuse by the controller
// when diffing observed labels against the spec.
func ToPtrSliceMap(in map[string][]string) map[string][]*string {
	if in == nil {
		return nil
	}
	out := make(map[string][]*string, len(in))
	for k, vals := range in {
		ptrs := make([]*string, 0, len(vals))
		for i := range vals {
			ptrs = append(ptrs, internal.Ptr(vals[i]))
		}
		out[k] = ptrs
	}
	return out
}

// observedLabels extracts the user-settable labels map from an observed
// instance, dereferencing the pointer safely and dropping SM-managed reserved
// keys so they never surface as spurious remove operations.
func observedLabels(observed *smopenapi.ServiceInstanceResponseObject) map[string][]string {
	if observed == nil || observed.Labels == nil {
		return nil
	}
	out := make(map[string][]string)
	for k, v := range *observed.Labels {
		if isReservedLabel(k) {
			continue
		}
		out[k] = v
	}
	return out
}

// normalizeValues sorts a copy of the values so comparison is order-independent.
func normalizeValues(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}

// diffLabels computes the operation-based label PATCH. For each desired key that
// is new or whose values changed, it emits an add op; for each observed key not
// present in the desired set, it emits a remove op.
func diffLabels(desired map[string][]*string, observed map[string][]string) []smopenapi.Label {
	desiredStr := toStringSliceMap(desired)

	// deterministic ordering of the emitted operations
	addKeys := make([]string, 0, len(desiredStr))
	for k := range desiredStr {
		addKeys = append(addKeys, k)
	}
	sort.Strings(addKeys)

	var ops []smopenapi.Label
	for _, k := range addKeys {
		want := normalizeValues(desiredStr[k])
		have, ok := observed[k]
		if ok && equalStringSlices(want, normalizeValues(have)) {
			continue
		}
		key := k
		ops = append(ops, smopenapi.Label{Key: &key, Op: internal.Ptr(labelOpAdd), Values: desiredStr[k]})
	}

	removeKeys := make([]string, 0, len(observed))
	for k := range observed {
		if _, ok := desiredStr[k]; !ok {
			removeKeys = append(removeKeys, k)
		}
	}
	sort.Strings(removeKeys)
	for _, k := range removeKeys {
		key := k
		ops = append(ops, smopenapi.Label{Key: &key, Op: internal.Ptr(labelOpRemove), Values: observed[k]})
	}

	return ops
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TimeToMetav1 converts a *time.Time (from the SM API) into a *metav1.Time for
// the CR status, returning nil for a nil input.
func TimeToMetav1(t *time.Time) *metav1.Time {
	if t == nil {
		return nil
	}
	mt := metav1.NewTime(*t)
	return &mt
}
