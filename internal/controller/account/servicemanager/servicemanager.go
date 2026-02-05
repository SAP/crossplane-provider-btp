package servicemanager

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	sm "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
		tracker:  c.resourcetracker,
		tfClient: tfClient,
		kube:     c.kube,
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

	return resStatus.ExternalObservation, err
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
