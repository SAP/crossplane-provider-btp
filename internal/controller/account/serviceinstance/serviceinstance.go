package serviceinstance

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	tjcontroller "github.com/crossplane/upjet/pkg/controller"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	siClient "github.com/sap/crossplane-provider-btp/internal/clients/account/serviceinstance"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"github.com/sap/crossplane-provider-btp/internal/features"
)

const (
	errNotServiceInstance = "managed resource is not a ServiceInstance custom resource"
	errTrackPCUsage       = "cannot track ProviderConfig usage"
	errGetPC              = "cannot get ProviderConfig"
	errGetCreds           = "cannot get credentials"

	errObserveInstance = "cannot observe serviceinstnace"
	errCreateInstance  = "cannot create serviceinstnace"
	errSaveData        = "cannot update cr data"
	errGetInstance     = "cannot get serviceinstnace"
)

var clientCreatorFn = func(kube client.Client) siClient.TfProxyClientCreator {
	return siClient.NewServiceInstanceClientCreator(
		// client defines what resource he needs a connector for, but here in DI we define how to create such tf connector
		func(resourceName string, gvk schema.GroupVersionKind, useAsync bool, callbackProvider tjcontroller.CallbackProvider) *tjcontroller.Connector {

			return tfclient.NewInternalTfConnector(kube, resourceName, gvk, useAsync, callbackProvider)
		},
		saveCallback,
		kube,
	)
}

// Setup adds a controller that reconciles ServiceInstance managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.ServiceInstanceGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.ServiceInstanceGroupVersionKind),
		managed.WithExternalConnecter(&connector{
			kube:  mgr.GetClient(),
			usage: resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),

			newClientCreatorFn: clientCreatorFn,
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1alpha1.ServiceInstance{}).
		WithEventFilter(resource.DesiredStateChanged()).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// SaveConditionsFn Callback for persisting conditions in the CR
var saveCallback siClient.SaveConditionsFn = func(ctx context.Context, kube client.Client, name string, conditions ...xpv1.Condition) error {

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

	newClientCreatorFn func(kube client.Client) siClient.TfProxyClientCreator
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	_, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return nil, errors.New(errNotServiceInstance)
	}

	// when working with tf proxy resources we want to keep the Connect() logic as part of the delgating Connect calls of the native resources to
	// deal with errors in the part of process that they belong to
	clientCreator := c.newClientCreatorFn(c.kube)

	client, err := clientCreator.Connect(ctx, mg.(*v1alpha1.ServiceInstance))
	if err != nil {
		return nil, err
	}

	return &external{tfClient: client, kube: c.kube}, nil
}

type external struct {
	tfClient siClient.TfProxyClient
	kube     client.Client
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotServiceInstance)
	}
	exists, err := e.tfClient.Observe(ctx)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetInstance)
	}
	if !exists {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	//TODO: add uodate logic
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
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
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

	return managed.ExternalUpdate{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	_, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return errors.New(errNotServiceInstance)
	}
	return nil
}

func (e *external) saveInstanceData(ctx context.Context, cr *v1alpha1.ServiceInstance, sid siClient.ServiceInstanceData) error {
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
