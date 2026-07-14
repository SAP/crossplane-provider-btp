package servicemanager

import (
	"context"
	"fmt"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/internal/adoption"
	sm "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apisv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotServiceManager = "managed resource is not a ServiceManager custom resource"
	errTrack             = "cannot track resource usage"
	errInitialize        = "while initializing service plan ID"
	errConnect           = "while connecting resources"
	errGetPlanID         = "while getting plan ID initializer"
	errUpdateStatus      = "while updating service manager status"
	errCreate            = "while creating resources"
	errUpdate            = "while updating resources"
	errDelete            = "while deleting resources"
	errSetStatus         = "while setting status"
	errGetServicePlan    = "while getting service manager plan ID by name"
)

// ServiceManagerPlanIdInitializer is will provide implementation of service plan id lookup by name
type ServiceManagerPlanIdInitializer interface {
	ServiceManagerPlanIDByName(ctx context.Context, subaccountId string, servicePlanName string) (string, error)
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube            client.Client
	resourcetracker tracking.ReferenceResolverTracker
	newServiceFn    func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)

	newPlanIdInitializerFn func(ctx context.Context, cr *apisv1beta1.ServiceManager) (ServiceManagerPlanIdInitializer, error)
	newClientInitalizerFn  func() sm.ITfClientInitializer

	// newAdminLookuperFn builds a SemanticLookuper backed by the subaccount-admin
	// SM binding (minted via the accounts-service), returning a cleanup func.
	newAdminLookuperFn func(ctx context.Context, cr *apisv1beta1.ServiceManager) (sm.SemanticLookuper, func(), error)
	// recorder emits Kubernetes events for the heal path. May be nil.
	recorder event.Recorder
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*apisv1beta1.ServiceManager)
	if !ok {
		return nil, errors.New(errNotServiceManager)
	}
	if err := c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrack)
	}

	if err := c.InitializeServicePlanId(ctx, cr); err != nil {
		return nil, errors.Wrap(err, errInitialize)
	}

	tfClientInit := c.newClientInitalizerFn()

	tfClient, err := tfClientInit.ConnectResources(ctx, cr)

	if err != nil {
		return nil, errors.Wrap(err, errConnect)
	}

	return &external{
		tracker:            c.resourcetracker,
		tfClient:           tfClient,
		kube:               c.kube,
		recorder:           c.recorder,
		newAdminLookuperFn: c.newAdminLookuperFn,
	}, nil
}

func (c *connector) IsInitialized(cr *apisv1beta1.ServiceManager) bool {
	return cr.Spec.ForProvider.SubaccountGuid != "" && cr.Status.AtProvider.DataSourceLookup != nil
}

func (c *connector) InitializeServicePlanId(ctx context.Context, cr *apisv1beta1.ServiceManager) error {
	if c.IsInitialized(cr) {
		return nil
	}

	planIdInitializer, err := c.newPlanIdInitializerFn(ctx, cr)
	if err != nil {
		return errors.Wrap(err, errGetPlanID)
	}

	id, err := planIdInitializer.ServiceManagerPlanIDByName(ctx, cr.Spec.ForProvider.SubaccountGuid, c.ServicePlanName(cr))
	if err != nil {
		return errors.Wrap(err, errGetServicePlan)
	}

	return c.saveId(ctx, cr, id)
}

func (c *connector) ServicePlanName(cr *apisv1beta1.ServiceManager) string {
	if cr.Spec.ForProvider.PlanName != "" {
		return cr.Spec.ForProvider.PlanName
	}
	return apisv1beta1.DefaultPlanName
}

func (c *connector) saveId(ctx context.Context, cr *apisv1beta1.ServiceManager, id string) error {
	cr.Status.AtProvider.DataSourceLookup = &apisv1beta1.DataSourceLookup{
		ServiceManagerPlanID: id,
	}
	if err := c.kube.Status().Update(ctx, cr); err != nil {
		return errors.Wrap(err, errUpdateStatus)
	}
	return nil
}

type external struct {
	kube    client.Client
	tracker tracking.ReferenceResolverTracker

	tfClient sm.ITfClient

	// newAdminLookuperFn builds the subaccount-admin-backed SemanticLookuper
	// (minted via the accounts-service), returning a cleanup func.
	newAdminLookuperFn func(ctx context.Context, cr *apisv1beta1.ServiceManager) (sm.SemanticLookuper, func(), error)
	// recorder emits Kubernetes events for the heal path. May be nil.
	recorder event.Recorder
}

// Disconnect is a no-op for the external client to close its connection.
// Since we dont need this, we only have it to fullfil the interface.
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*apisv1beta1.ServiceManager)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotServiceManager)
	}

	resStatus, err := c.tfClient.ObserveResources(ctx, cr)

	statusErr := c.setStatus(ctx, resStatus, cr)
	if statusErr != nil {
		return managed.ExternalObservation{}, errors.Wrap(statusErr, errSetStatus)
	}

	// Orphaned-external-name adoption: BTP has a resource matching this
	// managed CR, but our external-name is still a fallback (empty or ==
	// metadata.name) — Observe cannot resolve it and Delete would strip the
	// finalizer, orphaning the BTP resource. Try a semantic lookup by the
	// subaccount-admin plan (1-per-subaccount) and adopt on a unique match.
	//
	// NOTE: only fires on TRUE fallback. A single-UUID external-name is the
	// natural output of the operator's phase-1 Create (createInstance) — the
	// same reconcile loop is expected to run phase-2 (createBinding) next and
	// upgrade to the compound "<sID>/<bID>" key. Re-firing adoption in that
	// state used to trap SM in an infinite adoption loop; see adoption.go.
	if err == nil && !resStatus.ResourceExists &&
		adoption.IsFallbackExternalName(cr.Name, meta.GetExternalName(cr)) {
		if healErr := c.healExternalName(ctx, cr); healErr != nil {
			return managed.ExternalObservation{}, healErr
		}
	}

	return resStatus.ExternalObservation, err
}

// healExternalName performs the orphaned-external-name adoption for a
// ServiceManager. It runs a semantic lookup by the resolved subaccount-admin
// plan ID (1-per-subaccount) and, on a unique match, patches
// crossplane.io/external-name with the "<serviceInstanceID>/<serviceBindingID>"
// compound key.
//
// Return contract matches the ServiceInstance heal: ErrRequeueAfterAdopt on a
// successful adoption, nil when there is nothing to adopt or the lookup failed,
// a real error only when persisting fails.
func (c *external) healExternalName(ctx context.Context, cr *apisv1beta1.ServiceManager) error {
	if c.newAdminLookuperFn == nil {
		return nil
	}
	if cr.Status.AtProvider.DataSourceLookup == nil {
		return nil
	}
	planID := cr.Status.AtProvider.DataSourceLookup.ServiceManagerPlanID
	if planID == "" {
		return nil
	}

	lookuper, cleanup, err := c.newAdminLookuperFn(ctx, cr)
	if err != nil {
		log.FromContext(ctx).Info("external-name adoption: cannot obtain admin lookup client", "error", err.Error())
		c.emit(cr, event.Warning(event.Reason(adoption.EventReasonLookupFailed), err))
		return nil
	}
	defer cleanup()

	siID, sbID, instanceCreatedAt, found, err := lookuper.LookupInstanceAndBinding(ctx, planID, smInstanceName(cr), smBindingName(cr))
	if err != nil {
		log.FromContext(ctx).Info("external-name adoption lookup failed", "planID", planID, "error", err.Error())
		c.emit(cr, event.Warning(event.Reason(adoption.EventReasonLookupFailed), err))
		return nil
	}
	if !found {
		return nil
	}

	// Ownership check: refuse to adopt a service-manager instance that
	// predates our CR (brownfield). The check uses the instance's created_at
	// because we always create the instance first (phase-1); if the instance
	// isn't ours, the binding — which lives inside it — isn't either.
	if !adoption.IsOwnedByCR(cr.GetCreationTimestamp().Time, instanceCreatedAt) {
		log.FromContext(ctx).Info("external-name adoption refused: BTP service manager predates the CR (brownfield)",
			"serviceInstanceID", siID, "serviceBindingID", sbID, "planID", planID,
			"crCreatedAt", cr.GetCreationTimestamp().Time, "btpCreatedAt", instanceCreatedAt)
		c.emit(cr, event.Warning(
			event.Reason(adoption.EventReasonRefusedBrownfield),
			errors.Errorf(
				"refusing to adopt existing BTP service manager %s/%s: instance created_at %s predates the CR's creationTimestamp %s (brownfield). Set crossplane.io/external-name explicitly to import it (see external-name ADR)",
				siID, sbID, instanceCreatedAt.Format(time.RFC3339), cr.GetCreationTimestamp().Time.Format(time.RFC3339))))
		return nil
	}

	meta.SetExternalName(cr, formExternalName(siID, sbID))
	if uErr := c.kube.Update(ctx, cr); uErr != nil {
		return errors.Wrap(uErr, "cannot persist adopted external-name")
	}

	log.FromContext(ctx).Info("adopted existing BTP service manager by external-name", "serviceInstanceID", siID, "serviceBindingID", sbID, "planID", planID)
	c.emit(cr, event.Normal(event.Reason(adoption.EventReasonAdopted),
		fmt.Sprintf("Adopted existing BTP service manager %s/%s (semantic key: planID=%s, instance created_at=%s)", siID, sbID, planID, instanceCreatedAt.Format(time.RFC3339))))
	return adoption.ErrRequeueAfterAdopt
}

// emit records a Kubernetes event when a recorder is configured.
func (c *external) emit(cr resource.Managed, ev event.Event) {
	if c.recorder != nil {
		c.recorder.Event(cr, ev)
	}
}

// smInstanceName returns the managed service-manager instance name used to
// disambiguate the subaccount-admin plan (which also holds an access instance).
func smInstanceName(cr *apisv1beta1.ServiceManager) string {
	if cr.Spec.ForProvider.ServiceInstanceName != "" {
		return cr.Spec.ForProvider.ServiceInstanceName
	}
	return apisv1beta1.DefaultServiceInstanceName
}

// smBindingName returns the managed service-manager binding name so a transient
// admin binding is never adopted.
func smBindingName(cr *apisv1beta1.ServiceManager) string {
	if cr.Spec.ForProvider.ServiceBindingName != "" {
		return cr.Spec.ForProvider.ServiceBindingName
	}
	return apisv1beta1.DefaultServiceBindingName
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*apisv1beta1.ServiceManager)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotServiceManager)
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
	cr, ok := mg.(*apisv1beta1.ServiceManager)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotServiceManager)
	}

	err := c.tfClient.UpdateResources(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	return managed.ExternalUpdate{}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*apisv1beta1.ServiceManager)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotServiceManager)
	}

	cr.SetConditions(xpv1.Deleting())

	c.tracker.SetConditions(ctx, cr)

	if blocked := c.tracker.DeleteShouldBeBlocked(mg); blocked {
		return managed.ExternalDelete{}, errors.New(providerv1alpha1.ErrResourceInUse)
	}

	return managed.ExternalDelete{}, errors.Wrap(c.tfClient.DeleteResources(ctx, cr), errDelete)
}

func (c *external) setStatus(ctx context.Context, status sm.ResourcesStatus, cr *apisv1beta1.ServiceManager) error {
	if status.ResourceExists {
		cr.Status.SetConditions(xpv1.Available())
		cr.Status.AtProvider.Status = apisv1beta1.ServiceManagerBound
	} else {
		cr.Status.SetConditions(xpv1.Unavailable())
		cr.Status.AtProvider.Status = apisv1beta1.ServiceManagerUnbound
	}
	cr.Status.AtProvider.ServiceInstanceID = status.InstanceID
	cr.Status.AtProvider.ServiceBindingID = status.BindingID
	// Unfortunately we need to update the CR status manually here, because the reconciler will drop the change otherwise
	// (I guess because we are attempting to save something while ResourceExists remains false for another cycle)
	if err := c.kube.Status().Update(ctx, cr); err != nil {
		return errors.Wrap(err, errUpdateStatus)
	}
	return nil
}

// formExternalName forms an externalName from the given serviceInstanceID and serviceBindingID
func formExternalName(serviceInstanceID, serviceBindingID string) string {
	if serviceBindingID == "" {
		return serviceInstanceID
	}
	return serviceInstanceID + "/" + serviceBindingID
}
