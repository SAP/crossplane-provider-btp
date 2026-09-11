package serviceinstance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	ujresource "github.com/crossplane/upjet/v2/pkg/resource"

	"regexp"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	siClient "github.com/sap/crossplane-provider-btp/internal/clients/account/serviceinstance"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	tfClient "github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/di"
	"github.com/sap/crossplane-provider-btp/internal/recovery"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	errNotServiceInstance = "managed resource is not a ServiceInstance custom resource"
	errTrackPCUsage       = "cannot track ProviderConfig usage"
	errGetPC              = "cannot get ProviderConfig"
	errGetCreds           = "cannot get credentials"

	errObserveInstance = "cannot observe serviceinstance"
	errCreateInstance  = "cannot create serviceinstance"
	errUpdateInstance  = "cannot update serviceinstance"
	errSaveData        = "cannot update cr data"
	errGetInstance     = "cannot get serviceinstance"
	errTrackRUsage     = "cannot track ResourceUsage"
	errInitServicePlan = "while initializing service plan"
	errConnectClient   = "while connecting to service"
	errDeleteInstance  = "cannot delete serviceinstance"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Dependency Injection
var newClientCreatorFn = func(kube client.Client) tfClient.TfProxyConnectorI[*v1alpha1.ServiceInstance] {
	return siClient.NewServiceInstanceConnector(
		saveCallback,
		kube)
}

var newServicePlanInitializerFn = func() Initializer {
	return &servicePlanInitializer{
		newIdResolverFn: di.NewPlanIdResolverFn,
		loadSecretFn:    internal.LoadSecretData,
	}
}

// SaveConditionsFn Callback for persisting conditions in the CR
var saveCallback tfClient.SaveConditionsFn = func(ctx context.Context, kube client.Client, name string, conditions ...xpv1.Condition) error {

	si := &v1alpha1.ServiceInstance{}

	nn := types.NamespacedName{Name: name}
	if kErr := kube.Get(ctx, nn, si); kErr != nil {
		return errors.Wrap(kErr, errGetInstance)
	}

	// Store the CR's current generation on each condition so that Observe() can
	// detect whether the spec has changed since the async operation was triggered.
	for i := range conditions {
		conditions[i].ObservedGeneration = si.Generation
	}
	si.SetConditions(conditions...)

	uErr := kube.Status().Update(ctx, si)

	return errors.Wrap(uErr, errSaveData)
}

type connector struct {
	kube  client.Client
	usage providerconfig.LegacyTracker

	clientConnector             tfClient.TfProxyConnectorI[*v1alpha1.ServiceInstance]
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
	_, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return nil, errors.New(errNotServiceInstance)
	}
	// Skip ResourceUsage tracking when the MR is being deleted: Track() walks
	// references and Gets each upstream MR; if any was deleted out from under
	// us, that Get returns NotFound and Connect would abort before Delete()
	// runs, leaving the BTP-side instance and the finalizer in place forever.
	if !meta.WasDeleted(mg) {
		if err := c.resourcetracker.Track(ctx, mg); err != nil {
			return nil, errors.Wrap(err, errTrackRUsage)
		}
	}

	// we need to resolve the plan ID here, since at crossplanes initialize stage the required references for the sm secret are not resolved yet
	planInitializer := c.newServicePlanInitializerFn()
	err := planInitializer.Initialize(c.kube, ctx, mg)

	if err != nil {
		return nil, errors.Wrap(err, errInitServicePlan)
	}

	// when working with tf proxy resources we want to keep the Connect() logic as part of the delgating Connect calls of the native resources to
	// deal with errors in the part of process that they belong to
	client, err := c.clientConnector.Connect(ctx, mg.(*v1alpha1.ServiceInstance))
	if err != nil {
		return nil, errors.Wrap(err, errConnectClient)
	}

	ext := &external{tfClient: client, kube: c.kube, tracker: c.resourcetracker, recorder: c.recorder,
		newAdminLookuperFn: c.newAdminLookuperFn}

	return ext, nil
}

type external struct {
	tfClient tfClient.TfProxyControllerI
	kube     client.Client
	tracker  tracking.ReferenceResolverTracker

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

	// ADR(external-name): check for conflict condition from a previous failed Create()
	// Only block if the spec hasn't changed since the conflict (same Generation)
	lastAsyncOp := cr.GetCondition(xpv1.ConditionType(ujresource.TypeLastAsyncOperation))
	if lastAsyncOp.Status == corev1.ConditionFalse &&
		strings.Contains(lastAsyncOp.Message, "Conflict") &&
		lastAsyncOp.ObservedGeneration == cr.Generation {
		// Try recovery instead of forcing the ADR-prescribed error-loop path (see
		// docs/contribution-notes/external-name-handling.md).
		if recovery.IsFallbackExternalName(cr.Name, meta.GetExternalName(cr)) {
			if healErr := e.healExternalName(ctx, cr); healErr != nil {
				return managed.ExternalObservation{}, healErr
			}
		}
		return managed.ExternalObservation{ResourceExists: false},
			errors.New("creation failed - resource already exists. Please set external-name annotation to adopt the existing resource or change the name to create a new one")
	}

	// ADR(external-name): validate external-name is a UUID if set
	externalName := meta.GetExternalName(cr)
	if externalName != "" && externalName != cr.Name {
		if !isValidUUID(externalName) {
			return managed.ExternalObservation{},
				errors.New("external-name is not a valid UUID. Please check the value of the external-name annotation and set it to the ServiceInstance ID (UUID format) if you want to adopt an existing resource, or remove the annotation if you want to create a new one")
		}
	}

	status, details, err := e.tfClient.Observe(ctx)
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	//Check for failed async operations ONCE, before the switch
	if e.checkAsyncOperationFailure(cr) {
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: false,
		}, nil
	}

	switch status {
	case tfClient.NotExisting:
		// Recovery also covers the delete leg: healing here lets the next
		// reconcile's Delete() target the real BTP resource instead of
		// stripping the finalizer and orphaning it.
		if recovery.IsFallbackExternalName(cr.Name, meta.GetExternalName(cr)) {
			if healErr := e.healExternalName(ctx, cr); healErr != nil {
				return managed.ExternalObservation{}, healErr
			}
		}
		return managed.ExternalObservation{ResourceExists: false}, nil
	case tfClient.Drift:
		// ADR(external-name): Calculate and report diff between desired state and what was observed from the API
		diff := e.calculateDiff(cr)

		// ADR(external-name): Set condition with drift information so it appears in events
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
	case tfClient.UpToDate:

		data := e.tfClient.QueryAsyncData(ctx)

		if data != nil {
			// since its an async resource, we need to save the external-name in the observe()
			if err := e.saveInstanceData(ctx, cr, *data); err != nil {
				return managed.ExternalObservation{}, errors.Wrap(err, errSaveData)
			}
			// Only set Available condition if ManagementPolicy is not only "Observe", since Available condition sets Ready to True
			// and we don't want that for Observe-only resources
			if !isObserveOnly(cr) {
				cr.SetConditions(xpv1.Available())
			}
		}

		return managed.ExternalObservation{
			ResourceExists:    true,
			ResourceUpToDate:  true,
			ConnectionDetails: details,
		}, nil
	}
	return managed.ExternalObservation{}, errors.New(errObserveInstance)
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotServiceInstance)
	}

	// ADR(external-name): setting external-name not possible due to an async operation
	// After creation, external-name will be populated by Observe() in the next reconciliation
	// If creation fails with conflict, the AsyncOperation condition will be set by upjet's callback
	// and will be handled in the next Observe() call (see conflict detection logic above)

	cr.SetConditions(xpv1.Creating())
	if err := e.tfClient.Create(ctx); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateInstance)
	}

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	_, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotServiceInstance)
	}

	err := c.tfClient.Update(ctx)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateInstance)
	}

	return managed.ExternalUpdate{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotServiceInstance)
	}
	cr.SetConditions(xpv1.Deleting())

	// Set resource usage conditions to check dependencies
	c.tracker.SetConditions(ctx, cr)

	// Block deletion if other resources are still using this ServiceInstance
	if blocked := c.tracker.DeleteShouldBeBlocked(mg); blocked {
		return managed.ExternalDelete{}, errors.New(providerv1alpha1.ErrResourceInUse)
	}

	if err := c.tfClient.Delete(ctx); err != nil {
		// 404 not found, does not need to be handeled since already done by upjet/terrarform
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

	// Clear stale async-failure conditions BEFORE persisting the recovered
	// external-name. If either write fails, the CR still has a fallback
	// external-name and the heal re-runs on the next reconcile. Reversed, a
	// mid-way failure would leave the CR with a real external-name AND stale
	// conditions — a permanent stuck state (heal then skipped).
	clearAsyncFailureConditions(cr)
	if uErr := e.kube.Status().Update(ctx, cr); uErr != nil {
		return errors.Wrap(uErr, "cannot clear async-failure conditions before recovery")
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

// clearAsyncFailureConditions resets the stale LastAsyncOperation failure so a
// subsequent reconcile is not tricked back into the Conflict branch after the
// external-name has been recovered.
func clearAsyncFailureConditions(cr *v1alpha1.ServiceInstance) {
	cr.SetConditions(xpv1.Condition{
		Type:               xpv1.ConditionType(ujresource.TypeLastAsyncOperation),
		Status:             corev1.ConditionUnknown,
		LastTransitionTime: metav1.Now(),
		Reason:             "RecoveredExternalName",
		ObservedGeneration: cr.Generation,
	})
}

func (e *external) saveInstanceData(ctx context.Context, cr *v1alpha1.ServiceInstance, sid tfClient.ObservationData) error {
	if meta.GetExternalName(cr) != sid.ExternalName {
		meta.SetExternalName(cr, sid.ExternalName)
		// manually saving external-name, since crossplane reconciler won't update spec and status in one loop
		if err := e.kube.Update(ctx, cr); err != nil {
			return err
		}
	}
	// we rely on status being saved in crossplane reconciler here
	cr.Status.AtProvider.ID = sid.ID
	cr.Status.AtProvider.DashboardURL = sid.DashboardURL
	// Save additional observation fields from the Terraform resource
	cr.Status.AtProvider.CreatedDate = sid.CreatedDate
	cr.Status.AtProvider.LastModified = sid.LastModified
	cr.Status.AtProvider.State = sid.State
	cr.Status.AtProvider.Ready = sid.Ready
	cr.Status.AtProvider.Usable = sid.Usable
	cr.Status.AtProvider.PlatformID = sid.PlatformID
	return nil
}

// checkAsyncOperationFailure checks if there's a failed async operation and sets appropriate conditions
func (e *external) checkAsyncOperationFailure(cr *v1alpha1.ServiceInstance) bool {
	lastAsyncOp := cr.GetCondition(xpv1.ConditionType("LastAsyncOperation"))
	if lastAsyncOp.Status == corev1.ConditionFalse && lastAsyncOp.Reason == "ApplyFailure" {
		return true
	}

	// Also check AsyncOperation as fallback
	asyncOp := cr.GetCondition(ujresource.TypeAsyncOperation)
	if asyncOp.Status == corev1.ConditionFalse && asyncOp.Reason == "ApplyFailure" {
		return true
	}

	return false
}

func isObserveOnly(cr *v1alpha1.ServiceInstance) bool {
	policies := cr.GetManagementPolicies()
	return len(policies) == 1 && policies[0] == xpv1.ManagementActionObserve
}

// calculateDiff compares the desired state (spec) with the observed state from the API
// Returns a human-readable diff string following the ADR(external-name) requirement for drift reporting
func (e *external) calculateDiff(cr *v1alpha1.ServiceInstance) string {
	// Get the Terraform resource to access both desired and observed state
	tfResource := e.tfClient.GetTfResource()
	if tfResource == nil {
		return "Drift detected: unable to retrieve Terraform resource details"
	}

	// Type assert to SubaccountServiceInstance (the upjetted resource)
	upjettedSI, ok := tfResource.(*v1alpha1.SubaccountServiceInstance)
	if !ok {
		return fmt.Sprintf("Drift detected: unexpected resource type %T", tfResource)
	}

	// Build desired state from Spec.ForProvider (what user wants)
	desired := map[string]any{
		"name":           upjettedSI.Spec.ForProvider.Name,
		"subaccount_id":  upjettedSI.Spec.ForProvider.SubaccountID,
		"shared":         upjettedSI.Spec.ForProvider.Shared,
		"parameters":     upjettedSI.Spec.ForProvider.Parameters,
		"serviceplan_id": upjettedSI.Spec.ForProvider.ServiceplanID,
		"labels":         upjettedSI.Spec.ForProvider.Labels,
	}

	// Build observed state from Status.AtProvider (what API returned)
	observed := map[string]any{
		"name":           upjettedSI.Status.AtProvider.Name,
		"subaccount_id":  upjettedSI.Status.AtProvider.SubaccountID,
		"shared":         upjettedSI.Status.AtProvider.Shared,
		"parameters":     upjettedSI.Status.AtProvider.Parameters,
		"serviceplan_id": upjettedSI.Status.AtProvider.ServiceplanID,
		"labels":         upjettedSI.Status.AtProvider.Labels,
	}

	// Compare all fields between desired and observed state
	diff := cmp.Diff(desired, observed)

	if diff == "" {
		// If no structural diff found, check async operation message
		if asyncCond := cr.GetCondition(ujresource.TypeAsyncOperation); asyncCond.Message != "" {
			return fmt.Sprintf("Drift detected. Terraform message: %s", asyncCond.Message)
		}
		return "Drift detected: external resource differs from desired state"
	}

	return diff
}

func isValidUUID(s string) bool {
	return uuidRegex.MatchString(strings.ToLower(s))
}
