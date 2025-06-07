package serviceinstance

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	siClient "github.com/sap/crossplane-provider-btp/internal/clients/account/serviceinstance"
	tfClient "github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"github.com/sap/crossplane-provider-btp/internal/di"
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
)

// Dependency Injection
var newClientCreatorFn = func(kube client.Client) tfClient.TfProxyConnectorI[*v1alpha1.ServiceInstance] {
	return siClient.NewServiceInstanceConnector(
		saveCallback,
		kube)
}

var newServicePlanInitializerFn = func() Initializer {
	return &servicePlanInitializer{
		newIdResolverFn: di.NewPlanIdResolverFn,
		loadSecretFn:    di.LoadSecretData,
	}
}

// SaveConditionsFn Callback for persisting conditions in the CR
var saveCallback tfClient.SaveConditionsFn = func(ctx context.Context, kube client.Client, name string, conditions ...xpv1.Condition) error {

	si := &v1alpha1.ServiceInstance{}

	nn := types.NamespacedName{Name: name}
	if kErr := kube.Get(ctx, nn, si); kErr != nil {
		return errors.Wrap(kErr, errGetInstance)
	}

	si.SetConditions(conditions...)

	uErr := kube.Status().Update(ctx, si)

	return errors.Wrap(uErr, errSaveData)
}

type connector struct {
	kube  client.Client
	usage resource.Tracker

	clientConnector             tfClient.TfProxyConnectorI[*v1alpha1.ServiceInstance]
	newServicePlanInitializerFn func() Initializer
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	_, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return nil, errors.New(errNotServiceInstance)
	}

	// we need to resolve the plan ID here, since at crossplanes initialize stage the required references for the sm secret are not resolved yet
	planInitializer := c.newServicePlanInitializerFn()
	err := planInitializer.Initialize(c.kube, ctx, mg)

	if err != nil {
		return nil, err
	}

	// when working with tf proxy resources we want to keep the Connect() logic as part of the delgating Connect calls of the native resources to
	// deal with errors in the part of process that they belong to
	client, err := c.clientConnector.Connect(ctx, mg.(*v1alpha1.ServiceInstance))
	if err != nil {
		return nil, err
	}

	return &external{tfClient: client, kube: c.kube}, nil
}

type external struct {
	tfClient tfClient.TfProxyControllerI
	kube     client.Client
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotServiceInstance)
	}
	status, details, err := e.tfClient.Observe(ctx)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetInstance)
	}

	switch status {
	case tfClient.NotExisting:
		return managed.ExternalObservation{ResourceExists: false}, nil
	case tfClient.Drift:
		return managed.ExternalObservation{
			ResourceExists:    true,
			ResourceUpToDate:  false,
			ConnectionDetails: managed.ConnectionDetails{},
		}, nil
	case tfClient.UpToDate:
		data := e.tfClient.QueryAsyncData(ctx)

		if data != nil {
			if err := e.saveInstanceData(ctx, cr, *data); err != nil {
				return managed.ExternalObservation{}, errors.Wrap(err, errSaveData)
			}
			cr.SetConditions(xpv1.Available())
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

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return errors.New(errNotServiceInstance)
	}
	cr.SetConditions(xpv1.Deleting())
	if err := c.tfClient.Delete(ctx); err != nil {
		return errors.Wrap(err, "cannot delete serviceinstance")
	}
	return nil
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
	return nil
}
