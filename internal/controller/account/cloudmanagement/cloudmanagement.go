package cloudmanagement

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/recovery"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	apisv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	cmclient "github.com/sap/crossplane-provider-btp/internal/clients/cis"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotCloudManagement   = "managed resource is not a CloudManagement custom resource"
	errExtractSecretKey     = "No Service Manager Secret Found"
	errGetCredentialsSecret = "Could not Get Secret"
	errTrackRUsage          = "cannot track ResourceUsage"
	errTrackPCUsage         = "cannot track ProviderConfig usage"
	errInitServicePlanId    = "while initializing service plan ID"
	errEnsureCompatibility  = "while ensuring compatibility"
	errConnectResources     = "while connecting resources"
	errObserve              = "while observing resources"
	errSetStatus            = "while setting status"
	errCreate               = "while creating resources"
	errUpdate               = "while updating resources"
	errDelete               = "while deleting resources"
	errSaveId               = "while saving ID"
	errGetPlanId            = "while getting plan ID"
)

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube                client.Client
	usage               providerconfig.LegacyTracker
	resourcetracker     tracking.ReferenceResolverTracker
	newPlanIdResolverFn func(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error)

	newClientInitalizerFn func() cmclient.ITfClientInitializer

	// newAdminLookuperFn builds a SemanticLookuper backed by the subaccount-admin
	// SM binding (via the accounts-service), returning a cleanup func.
	newAdminLookuperFn func(ctx context.Context, cr *apisv1beta1.CloudManagement) (servicemanager.SemanticLookuper, func(), error)
	// recorder emits Kubernetes events for the heal path. May be nil.
	recorder event.Recorder
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*apisv1beta1.CloudManagement)

	if !ok {
		return nil, errors.New(errNotCloudManagement)
	}

	if err := c.usage.Track(ctx, mg.(providerconfig.LegacyManaged)); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
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

	if cr.Spec.ForProvider.ServiceManagerSecret == "" || cr.Spec.ForProvider.ServiceManagerSecretNamespace == "" {
		return nil, errors.New(errExtractSecretKey)
	}
	secret := &corev1.Secret{}
	if err := c.kube.Get(
		ctx, types.NamespacedName{
			Namespace: cr.Spec.ForProvider.ServiceManagerSecretNamespace,
			Name:      cr.Spec.ForProvider.ServiceManagerSecret,
		}, secret,
	); err != nil {
		return nil, errors.Wrap(err, errGetCredentialsSecret)
	}

	err := c.InitializeServicePlanId(ctx, cr, secret)
	if err != nil {
		return nil, errors.Wrap(err, errInitServicePlanId)
	}

	err = c.ensureCompatibility(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, errEnsureCompatibility)
	}

	tfClientInit := c.newClientInitalizerFn()
	tfClient, err := tfClientInit.ConnectResources(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, errConnectResources)
	}

	ext := &external{
		kube:               c.kube,
		tracker:            c.resourcetracker,
		tfClient:           tfClient,
		recorder:           c.recorder,
		newAdminLookuperFn: c.newAdminLookuperFn,
	}

	return ext, nil
}

func (c *connector) ensureCompatibility(ctx context.Context, cr *apisv1beta1.CloudManagement) error {
	if c.migrationNeeded(cr) {
		ctrl.Log.Info(fmt.Sprintf("Migrating external-name to new format for cloudmanagement resource %v", cr.Name))
		meta.SetExternalName(cr,
			formExternalName(
				internal.Val(cr.Status.AtProvider.Instance.Id),
				internal.Val(cr.Status.AtProvider.Binding.Id),
			),
		)
		return c.kube.Update(ctx, cr)
	}
	return nil
}

func (c *connector) migrationNeeded(cr *apisv1beta1.CloudManagement) bool {
	extName := meta.GetExternalName(cr)
	instance := cr.Status.AtProvider.Instance
	binding := cr.Status.AtProvider.Binding

	return !strings.Contains(extName, "/") && instance != nil && binding != nil
}

func (c *connector) IsInitialized(cr *apisv1beta1.CloudManagement) bool {
	return cr.Status.AtProvider.DataSourceLookup != nil
}

// InitializeServicePlanId ensures the service plan id for cis local is cached in status
func (c *connector) InitializeServicePlanId(ctx context.Context, cr *apisv1beta1.CloudManagement, secret *corev1.Secret) error {
	if c.IsInitialized(cr) {
		return nil
	}

	sm, err := c.newPlanIdResolverFn(ctx, secret.Data)
	if err != nil {
		return errors.Wrap(err, errGetPlanId)
	}

	id, err := sm.PlanIDByName(ctx, "cis", "local", "")
	if err != nil {
		return errors.Wrap(err, errGetPlanId)
	}

	return errors.Wrap(c.saveId(ctx, cr, id), errSaveId)
}

func (c *connector) saveId(ctx context.Context, cr *apisv1beta1.CloudManagement, id string) error {
	cr.Status.AtProvider.DataSourceLookup = &apisv1beta1.CloudManagementDataSourceLookup{
		CloudManagementPlanID: id,
	}
	return c.kube.Status().Update(ctx, cr)
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	kube    client.Client
	tracker tracking.ReferenceResolverTracker

	tfClient cmclient.ITfClient

	// newAdminLookuperFn builds the subaccount-admin-backed SemanticLookuper.
	newAdminLookuperFn func(ctx context.Context, cr *apisv1beta1.CloudManagement) (servicemanager.SemanticLookuper, func(), error)
	// recorder emits Kubernetes events for the heal path. May be nil.
	recorder event.Recorder
}

// Disconnect is a no-op for the external client to close its connection.
// Since we dont need this, we only have it to fullfil the interface.
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*apisv1beta1.CloudManagement)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotCloudManagement)
	}

	resStatus, err := c.tfClient.ObserveResources(ctx, cr)

	statusErr := c.setStatus(ctx, resStatus, cr)
	if statusErr != nil {
		return managed.ExternalObservation{}, errors.Wrap(statusErr, errSetStatus)
	}

	// Recovery: BTP has a resource matching this managed CR, but our
	// external-name is still a fallback — semantic lookup + ownership check
	// (see internal/recovery). Fires only on TRUE fallback: a single-UUID
	// external-name is the natural output of phase-1 Create and must NOT
	// re-trigger recovery (would trap CM in an infinite loop before phase-2).
	if err == nil && !resStatus.ResourceExists &&
		recovery.IsFallbackExternalName(cr.Name, meta.GetExternalName(cr)) {
		if healErr := c.healExternalName(ctx, cr); healErr != nil {
			return managed.ExternalObservation{}, healErr
		}
	}

	return resStatus.ExternalObservation, errors.Wrap(err, errObserve)
}

func (c *external) healExternalName(ctx context.Context, cr *apisv1beta1.CloudManagement) error {
	if c.newAdminLookuperFn == nil {
		return nil
	}
	if !recovery.HasCreateBeenAttempted(cr) {
		return nil
	}
	if cr.Status.AtProvider.DataSourceLookup == nil {
		return nil
	}
	planID := cr.Status.AtProvider.DataSourceLookup.CloudManagementPlanID
	if planID == "" {
		return nil
	}

	lookuper, cleanup, err := c.newAdminLookuperFn(ctx, cr)
	if err != nil {
		log.FromContext(ctx).Info("external-name recovery: cannot obtain admin lookup client", "error", err.Error())
		c.emit(cr, event.Warning(event.Reason(recovery.EventReasonLookupFailed), err))
		return nil
	}
	defer cleanup()

	siID, sbID, instanceCreatedAt, found, err := lookuper.LookupInstanceAndBinding(ctx, planID, cmInstanceName(cr), cmBindingName(cr))
	if err != nil {
		log.FromContext(ctx).Info("external-name recovery lookup failed", "planID", planID, "error", err.Error())
		c.emit(cr, event.Warning(event.Reason(recovery.EventReasonLookupFailed), err))
		return nil
	}
	if !found {
		return nil
	}

	// Uses the instance's created_at (phase-1 creates the instance first — if
	// the instance isn't ours, the binding inside it isn't either).
	if !recovery.IsOwnedByCR(cr, instanceCreatedAt) {
		log.FromContext(ctx).Info("external-name recovery refused: BTP cloud management is outside our Create-attempt window (brownfield)",
			"serviceInstanceID", siID, "serviceBindingID", sbID, "planID", planID,
			"crCreatedAt", cr.GetCreationTimestamp().Time, "btpCreatedAt", instanceCreatedAt)
		c.emit(cr, event.Warning(
			event.Reason(recovery.EventReasonRefusedBrownfield),
			errors.Errorf(
				"refusing to recover existing BTP cloud management %s/%s: instance created_at %s is outside the window where our own Create() attempt for this CR could have produced it (brownfield). Set crossplane.io/external-name explicitly to import it (see external-name ADR)",
				siID, sbID, instanceCreatedAt.Format(time.RFC3339))))
		return nil
	}

	meta.SetExternalName(cr, formExternalName(siID, sbID))
	if uErr := c.kube.Update(ctx, cr); uErr != nil {
		return errors.Wrap(uErr, "cannot persist recovered external-name")
	}

	log.FromContext(ctx).Info("recovered existing BTP cloud management by external-name", "serviceInstanceID", siID, "serviceBindingID", sbID, "planID", planID)
	c.emit(cr, event.Normal(event.Reason(recovery.EventReasonRecovered),
		fmt.Sprintf("Recovered existing BTP cloud management %s/%s (semantic key: planID=%s, instance created_at=%s)", siID, sbID, planID, instanceCreatedAt.Format(time.RFC3339))))
	return recovery.ErrRequeueAfterRecovery
}

// emit records a Kubernetes event when a recorder is configured.
func (c *external) emit(cr resource.Managed, ev event.Event) {
	if c.recorder != nil {
		c.recorder.Event(cr, ev)
	}
}

// cmInstanceName returns the managed cloud-management instance name used to
// disambiguate the cis/local plan.
func cmInstanceName(cr *apisv1beta1.CloudManagement) string {
	if cr.Spec.ForProvider.ServiceInstanceName != "" {
		return cr.Spec.ForProvider.ServiceInstanceName
	}
	return apisv1beta1.DefaultCloudManagementInstanceName
}

// cmBindingName returns the managed cloud-management binding name so a transient
// admin binding is never selected.
func cmBindingName(cr *apisv1beta1.CloudManagement) string {
	if cr.Spec.ForProvider.ServiceBindingName != "" {
		return cr.Spec.ForProvider.ServiceBindingName
	}
	return apisv1beta1.DefaultCloudManagementBindingName
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*apisv1beta1.CloudManagement)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotCloudManagement)
	}

	cr.SetConditions(xpv1.Creating())

	sID, bID, err := c.tfClient.CreateResources(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}
	meta.SetExternalName(cr, formExternalName(sID, bID))

	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*apisv1beta1.CloudManagement)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotCloudManagement)
	}

	err := c.tfClient.UpdateResources(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*apisv1beta1.CloudManagement)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotCloudManagement)
	}

	cr.SetConditions(xpv1.Deleting())

	c.tracker.SetConditions(ctx, cr)

	if blocked := c.tracker.DeleteShouldBeBlocked(mg); blocked {
		return managed.ExternalDelete{}, errors.New(providerv1alpha1.ErrResourceInUse)
	}

	return managed.ExternalDelete{}, errors.Wrap(c.tfClient.DeleteResources(ctx, cr), errDelete)
}

func (c *external) setStatus(ctx context.Context, status cmclient.ResourcesStatus, cr *apisv1beta1.CloudManagement) error {
	switch {
	case meta.WasDeleted(cr) && status.ResourceExists:
		// Mid-deletion, external instance still present. Status-only cosmetic:
		// finalization is gated on ExternalObservation.ResourceExists, not on
		// this condition. But without this branch the CR flips back to
		// Available/Bound between the Delete() call (which set Deleting()) and
		// the reconcile that finally observes the instance gone, which is
		// misleading in kubectl output and dashboards.
		cr.Status.SetConditions(xpv1.Deleting())
		cr.Status.AtProvider.Status = apisv1beta1.CisStatusUnbound
	case status.ResourceExists:
		cr.Status.SetConditions(xpv1.Available())
		cr.Status.AtProvider.Status = apisv1beta1.CisStatusBound
	default:
		cr.Status.SetConditions(xpv1.Unavailable())
		cr.Status.AtProvider.Status = apisv1beta1.CisStatusUnbound
	}

	if status.Instance.ID != nil {
		cr.Status.AtProvider.Instance = mapToInstance(&status.Instance)
		cr.Status.AtProvider.ServiceInstanceID = *status.Instance.ID
	}

	if status.Binding.ID != nil {
		cr.Status.AtProvider.Binding = mapToBinding(&status.Binding)
		cr.Status.AtProvider.ServiceBindingID = *status.Binding.ID
	}
	// Unfortunately we need to update the CR status manually here, because the reconciler will drop the change otherwise
	// (I guess because we are attempting to save something while ResourceExists remains false for another cycle)
	return c.kube.Status().Update(ctx, cr)
}

// formExternalName forms an externalName from the given serviceInstanceID and serviceBindingID
func formExternalName(serviceInstanceID, serviceBindingID string) string {
	if serviceBindingID == "" {
		return serviceInstanceID
	}
	return serviceInstanceID + "/" + serviceBindingID
}

func mapToInstance(src *apisv1alpha1.SubaccountServiceInstanceObservation) *apisv1beta1.Instance {
	if src == nil {
		return nil
	}

	return &apisv1beta1.Instance{
		Id:                   src.ID,
		Ready:                src.Ready,
		Name:                 src.Name,
		ServicePlanId:        src.ServiceplanID,
		PlatformId:           src.PlatformID,
		DashboardUrl:         src.DashboardURL,
		ReferencedInstanceId: src.ReferencedInstanceID,
		Shared:               src.Shared,
		Context:              unmarshalContext(src.Context),
		MaintenanceInfo:      nil,
		Usable:               src.Usable,
		CreatedAt:            src.CreatedDate,
		UpdatedAt:            src.LastModified,
		Labels:               nil,
	}
}

func mapToBinding(src *apisv1alpha1.SubaccountServiceBindingObservation) *apisv1beta1.Binding {
	if src == nil {
		return nil
	}

	return &apisv1beta1.Binding{
		Id:                src.ID,
		Ready:             src.Ready,
		Name:              src.Name,
		ServiceInstanceId: src.ServiceInstanceID,
		Context:           unmarshalContext(src.Context),
		BindResource:      nil,
		CreatedAt:         src.CreatedDate,
		UpdatedAt:         src.LastModified,
		Labels:            nil,
	}
}

func unmarshalContext(src *string) *map[string]string {
	if src == nil {
		return nil
	}

	var contextData map[string]string
	if err := json.Unmarshal([]byte(*src), &contextData); err != nil {
		return nil
	}
	return &contextData
}
