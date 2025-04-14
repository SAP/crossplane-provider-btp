package servicemanager

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sm "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"

	apisv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotServiceManager    = "managed resource is not a ServiceManager custom resource"
	errUpdateNotImplemented = "Update action not implemented"
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

	err := c.InitializeServicePlanId(ctx, cr)
	if err != nil {
		return nil, err
	}

	tfClientInit := c.newClientInitalizerFn()

	tfClient, err := tfClientInit.ConnectResources(ctx, cr)

	if err != nil {
		return nil, err
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
		return err
	}

	id, err := planIdInitializer.ServiceManagerPlanIDByName(ctx, cr.Spec.ForProvider.SubaccountGuid, c.ServicePlanName(cr))
	if err != nil {
		return err
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
	return c.kube.Status().Update(ctx, cr)
}

type external struct {
	kube    client.Client
	tracker tracking.ReferenceResolverTracker

	tfClient sm.ITfClient
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*apisv1beta1.ServiceManager)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotServiceManager)
	}

	resStatus, err := c.tfClient.ObserveResources(ctx, cr)

	statusErr := c.setStatus(ctx, resStatus, cr)
	if statusErr != nil {
		return managed.ExternalObservation{}, statusErr
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
		return managed.ExternalCreation{}, err
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
		return managed.ExternalUpdate{}, err
	}

	return managed.ExternalUpdate{}, err
}

func (c *external) Disconnect(ctx context.Context) error {
	// No-op
	return nil
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

	return managed.ExternalDelete{}, c.tfClient.DeleteResources(ctx, cr)
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
	return c.kube.Status().Update(ctx, cr)
}

// formExternalName forms an externalName from the given serviceInstanceID and serviceBindingID
func formExternalName(serviceInstanceID, serviceBindingID string) string {
	if serviceBindingID == "" {
		return serviceInstanceID
	}
	return serviceInstanceID + "/" + serviceBindingID
}
