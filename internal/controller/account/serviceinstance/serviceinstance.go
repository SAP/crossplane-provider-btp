package serviceinstance

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	siClient "github.com/sap/crossplane-provider-btp/internal/clients/account/serviceinstance"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/di"
	smopenapi "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
	"github.com/sap/crossplane-provider-btp/internal/recovery"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotServiceInstance = "managed resource is not a ServiceInstance custom resource"
	errGetCreds           = "cannot get credentials"

	errObserveInstance = "cannot observe serviceinstance"
	errCreateInstance  = "cannot create serviceinstance"
	errUpdateInstance  = "cannot update serviceinstance"
	errSaveData        = "cannot update cr data"
	errTrackRUsage     = "cannot track ResourceUsage"
	errInitServicePlan = "while initializing service plan"
	errConnectClient   = "while connecting to service"
	errDeleteInstance  = "cannot delete serviceinstance"
	errBuildParameters = "cannot build parameters"

	// Service Manager LastOperation states.
	opStateInProgress = "in progress"
	opStateSucceeded  = "succeeded"
	opStateFailed     = "failed"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Dependency Injection

var newServicePlanInitializerFn = func() Initializer {
	return &servicePlanInitializer{
		newIdResolverFn: di.NewPlanIdResolverFn,
		loadSecretFn:    internal.LoadSecretData,
	}
}

type connector struct {
	kube  client.Client
	usage providerconfig.LegacyTracker

	newServiceInstanceClientFn func(ctx context.Context, cr *v1alpha1.ServiceInstance) (siClient.ServiceInstanceClientI, error)

	newServicePlanInitializerFn func() Initializer
	resourcetracker             tracking.ReferenceResolverTracker

	// newAdminLookuperFn uses a subaccount-admin SM binding. Per-resource
	// serviceManagerSecret bindings are platform-scoped and do NOT list
	// instances created via the btp terraform provider.
	newAdminLookuperFn func(ctx context.Context, cr *v1alpha1.ServiceInstance) (smClient.SemanticLookuper, func(), error)
	// recorder emits Kubernetes events for the heal path. May be nil.
	recorder event.Recorder
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return nil, errors.New(errNotServiceInstance)
	}
	if err := c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackRUsage)
	}

	// we need to resolve the plan ID here, since at crossplanes initialize stage the required references for the sm secret are not resolved yet
	planInitializer := c.newServicePlanInitializerFn()
	if err := planInitializer.Initialize(c.kube, ctx, mg); err != nil {
		return nil, errors.Wrap(err, errInitServicePlan)
	}

	siClientImpl, err := c.newServiceInstanceClientFn(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, errConnectClient)
	}

	ext := &external{
		client:             siClientImpl,
		kube:               c.kube,
		tracker:            c.resourcetracker,
		recorder:           c.recorder,
		newAdminLookuperFn: c.newAdminLookuperFn,
	}

	return ext, nil
}

type external struct {
	client  siClient.ServiceInstanceClientI
	kube    client.Client
	tracker tracking.ReferenceResolverTracker

	// newAdminLookuperFn builds the subaccount-admin-backed SemanticLookuper.
	newAdminLookuperFn func(ctx context.Context, cr *v1alpha1.ServiceInstance) (smClient.SemanticLookuper, func(), error)
	// recorder emits Kubernetes events for the heal path. May be nil.
	recorder event.Recorder
}

// Disconnect is a no-op for the external client to close its connection.
// Since we dont need this, we only have it to fullfil the interface.
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotServiceInstance)
	}

	externalName := meta.GetExternalName(cr)

	// ADR(external-name): if external-name is still the fallback (== metadata.name),
	// the instance has not been created yet (or Create failed before persisting the
	// GUID). Try recovery; otherwise report not-existing so Create runs.
	if externalName == "" || recovery.IsFallbackExternalName(cr.Name, externalName) {
		if recovery.IsFallbackExternalName(cr.Name, externalName) {
			if healErr := e.healExternalName(ctx, cr); healErr != nil {
				return managed.ExternalObservation{}, healErr
			}
		}
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// ADR(external-name): validate external-name is a UUID if set
	if !isValidUUID(externalName) {
		return managed.ExternalObservation{},
			errors.New("external-name is not a valid UUID. Please check the value of the external-name annotation and set it to the ServiceInstance ID (UUID format) if you want to adopt an existing resource, or remove the annotation if you want to create a new one")
	}

	res, err := e.client.Observe(ctx, externalName)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserveInstance)
	}

	if !res.Exists {
		// Recovery also covers the delete leg: healing here lets the next
		// reconcile's Delete() target the real BTP resource instead of
		// stripping the finalizer and orphaning it.
		if recovery.IsFallbackExternalName(cr.Name, meta.GetExternalName(cr)) {
			if healErr := e.healExternalName(ctx, cr); healErr != nil {
				return managed.ExternalObservation{}, healErr
			}
		}
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	instance := res.Instance

	// Async poll: react to the LastOperation state.
	switch lastOperationState(instance) {
	case opStateInProgress:
		// A provision/update/delete is still running on the instance. We must
		// report ResourceUpToDate=true here: returning false would make the
		// crossplane reconciler call Update() while the operation is in flight,
		// which the Service Manager rejects with ConcurrentOperationInProgress.
		// Reporting up-to-date suppresses the spurious Update; crossplane
		// requeues via the poll interval and we re-observe until it settles.
		//
		// Ready reflects reality: an instance that is not yet usable is still
		// provisioning (Creating); one that is already usable is merely being
		// updated, so we leave its existing Ready condition untouched rather
		// than falsely downgrading it.
		if !isObserveOnly(cr) && !internal.Val(instance.Ready) {
			cr.SetConditions(xpv1.Creating())
		}
		return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
	case opStateFailed:
		cr.SetConditions(xpv1.Condition{
			Type:               xpv1.TypeReady,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Reason:             "OperationFailed",
			Message:            lastOperationMessage(instance),
		})
		return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false}, nil
	}

	// succeeded / Ready -> map status and compute drift.
	if err := e.saveInstanceData(ctx, cr, instance); err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errSaveData)
	}

	if !isObserveOnly(cr) && internal.Val(instance.Ready) {
		cr.SetConditions(xpv1.Available())
	}

	diff := e.calculateDiff(cr, instance)
	if diff != "" {
		cr.SetConditions(xpv1.Condition{
			Type:               xpv1.TypeReady,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Reason:             "DriftDetected",
			Message:            fmt.Sprintf("Drift detected: %s", diff),
		})
		return managed.ExternalObservation{
			ResourceExists:    true,
			ResourceUpToDate:  false,
			ConnectionDetails: managed.ConnectionDetails{},
			Diff:              diff,
		}, nil
	}

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotServiceInstance)
	}

	cr.SetConditions(xpv1.Creating())

	params, err := siClient.BuildComplexParameterMap(ctx, e.kube, cr.Spec.ForProvider.ParameterSecretRefs, cr.Spec.ForProvider.Parameters.Raw)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errBuildParameters)
	}

	id, err := e.client.Create(ctx, cr, params)
	if err != nil {
		// ADR(external-name): on Create error, leave external-name as fallback so
		// recovery can adopt a phantom-success instance on the next Observe.
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateInstance)
	}

	// Async create returned the new GUID: persist it as external-name (the
	// crossplane reconciler won't update spec and status in one loop).
	meta.SetExternalName(cr, id)
	if err := e.kube.Update(ctx, cr); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateInstance)
	}

	return managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{}}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotServiceInstance)
	}

	externalName := meta.GetExternalName(cr)

	params, err := siClient.BuildComplexParameterMap(ctx, e.kube, cr.Spec.ForProvider.ParameterSecretRefs, cr.Spec.ForProvider.Parameters.Raw)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errBuildParameters)
	}

	// Fetch the current instance so labels can be diffed into add/remove ops.
	res, err := e.client.Observe(ctx, externalName)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateInstance)
	}
	var observed *smopenapi.ServiceInstanceResponseObject
	if res.Exists {
		observed = res.Instance
	}

	if err := e.client.Update(ctx, externalName, cr, params, observed); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateInstance)
	}

	return managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{}}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotServiceInstance)
	}
	cr.SetConditions(xpv1.Deleting())

	// Set resource usage conditions to check dependencies
	e.tracker.SetConditions(ctx, cr)

	// Block deletion if other resources are still using this ServiceInstance
	if blocked := e.tracker.DeleteShouldBeBlocked(mg); blocked {
		return managed.ExternalDelete{}, errors.New(providerv1alpha1.ErrResourceInUse)
	}

	if err := e.client.Delete(ctx, meta.GetExternalName(cr)); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteInstance)
	}
	return managed.ExternalDelete{}, nil
}

func (e *external) healExternalName(ctx context.Context, cr *v1alpha1.ServiceInstance) error {
	if e.newAdminLookuperFn == nil {
		return nil
	}
	if !recovery.HasCreateBeenAttempted(cr) {
		return nil
	}
	lookuper, cleanup, err := e.newAdminLookuperFn(ctx, cr)
	if err != nil {
		log.FromContext(ctx).Info("external-name recovery: cannot obtain admin lookup client", "error", err.Error())
		e.emit(cr, event.Warning(event.Reason(recovery.EventReasonLookupFailed), err))
		return nil
	}
	defer cleanup()

	name := cr.Spec.ForProvider.Name
	guid, createdAt, found, err := lookuper.LookupServiceInstance(ctx, name)
	if err != nil {
		log.FromContext(ctx).Info("external-name recovery lookup failed", "name", name, "error", err.Error())
		e.emit(cr, event.Warning(event.Reason(recovery.EventReasonLookupFailed), err))
		return nil
	}
	if !found {
		return nil
	}

	if !recovery.IsOwnedByCR(cr, createdAt) {
		log.FromContext(ctx).Info("external-name recovery refused: BTP service instance is outside our Create-attempt window (brownfield)",
			"name", name, "guid", guid,
			"crCreatedAt", cr.GetCreationTimestamp().Time, "btpCreatedAt", createdAt)
		e.emit(cr, event.Warning(
			event.Reason(recovery.EventReasonRefusedBrownfield),
			errors.Errorf(
				"refusing to recover existing BTP service instance %s: created_at %s is outside the window where our own Create() attempt for this CR could have produced it (brownfield). Set crossplane.io/external-name explicitly to import it (see external-name ADR)",
				guid, createdAt.Format(time.RFC3339))))
		return nil
	}

	meta.SetExternalName(cr, guid)
	if uErr := e.kube.Update(ctx, cr); uErr != nil {
		return errors.Wrap(uErr, "cannot persist recovered external-name")
	}

	log.FromContext(ctx).Info("recovered existing BTP service instance by external-name", "guid", guid, "name", name)
	e.emit(cr, event.Normal(event.Reason(recovery.EventReasonRecovered),
		fmt.Sprintf("Recovered existing BTP service instance %s (semantic key: name=%s, created_at=%s)", guid, name, createdAt.Format(time.RFC3339))))
	return recovery.ErrRequeueAfterRecovery
}

// emit records a Kubernetes event when a recorder is configured.
func (e *external) emit(cr resource.Managed, ev event.Event) {
	if e.recorder != nil {
		e.recorder.Event(cr, ev)
	}
}

// saveInstanceData maps the observed Service Manager instance into the CR status.
func (e *external) saveInstanceData(ctx context.Context, cr *v1alpha1.ServiceInstance, instance *smopenapi.ServiceInstanceResponseObject) error {
	cr.Status.AtProvider.ID = instance.GetId()
	cr.Status.AtProvider.DashboardURL = instance.GetDashboardUrl()
	cr.Status.AtProvider.CreatedDate = siClient.TimeToMetav1(instance.CreatedAt)
	cr.Status.AtProvider.LastModified = siClient.TimeToMetav1(instance.UpdatedAt)
	cr.Status.AtProvider.State = lastOperationState(instance)
	cr.Status.AtProvider.Ready = instance.Ready
	cr.Status.AtProvider.Usable = instance.Usable
	cr.Status.AtProvider.PlatformID = instance.GetPlatformId()
	cr.Status.AtProvider.Shared = instance.Shared
	// ServiceplanID stays as resolved by the initializer; we rely on the
	// crossplane reconciler to persist the status here.
	return nil
}

func isObserveOnly(cr *v1alpha1.ServiceInstance) bool {
	policies := cr.GetManagementPolicies()
	return len(policies) == 1 && policies[0] == xpv1.ManagementActionObserve
}

// calculateDiff compares the desired spec against the observed Service Manager
// instance (GET-only). parameters and subaccount_id are intentionally excluded:
// the SM GET does not return parameters, and subaccount_id is immutable and not
// present on the instance GET. Returns "" when there is no drift.
func (e *external) calculateDiff(cr *v1alpha1.ServiceInstance, instance *smopenapi.ServiceInstanceResponseObject) string {
	desired := map[string]any{
		"name":            cr.Spec.ForProvider.Name,
		"service_plan_id": cr.Status.AtProvider.ServiceplanID,
		"labels":          normalizeLabels(siClient.FilterReservedLabels(cr.Spec.ForProvider.Labels)),
	}
	observed := map[string]any{
		"name":            instance.GetName(),
		"service_plan_id": instance.GetServicePlanId(),
		"labels":          normalizeLabels(observedSpecLabels(instance)),
	}

	// shared only drifts when it's managed (spec.Shared != nil).
	if cr.Spec.ForProvider.Shared != nil {
		desired["shared"] = internal.Val(cr.Spec.ForProvider.Shared)
		observed["shared"] = instance.GetShared()
	}

	return cmp.Diff(desired, observed)
}

// observedSpecLabels converts the observed instance labels back into the
// spec label shape (map[string][]*string) for comparison, dropping SM-managed
// reserved keys (e.g. subaccount_id) so they don't register as spurious drift.
func observedSpecLabels(instance *smopenapi.ServiceInstanceResponseObject) map[string][]*string {
	if instance == nil || instance.Labels == nil {
		return nil
	}
	return siClient.FilterReservedLabels(siClient.ToPtrSliceMap(*instance.Labels))
}

// normalizeLabels returns an order-independent, nil-dropped representation of
// the labels for comparison (sorted values per key).
func normalizeLabels(in map[string][]*string) map[string][]string {
	if len(in) == 0 {
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
		sort.Strings(strs)
		out[k] = strs
	}
	return out
}

func lastOperationState(instance *smopenapi.ServiceInstanceResponseObject) string {
	if instance == nil || instance.LastOperation == nil {
		return ""
	}
	return instance.LastOperation.GetState()
}

func lastOperationMessage(instance *smopenapi.ServiceInstanceResponseObject) string {
	if instance == nil || instance.LastOperation == nil {
		return "operation failed"
	}
	if desc := instance.LastOperation.GetDescription(); desc != "" {
		return desc
	}
	return "operation failed"
}

func isValidUUID(s string) bool {
	return uuidRegex.MatchString(strings.ToLower(s))
}
