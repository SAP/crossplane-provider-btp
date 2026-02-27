package serviceinstance

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ujresource "github.com/crossplane/upjet/pkg/resource"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	siClient "github.com/sap/crossplane-provider-btp/internal/clients/account/serviceinstance"
	tfClient "github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"github.com/sap/crossplane-provider-btp/internal/di"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
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

	si.SetConditions(conditions...)

	uErr := kube.Status().Update(ctx, si)

	return errors.Wrap(uErr, errSaveData)
}

type connector struct {
	kube  client.Client
	usage resource.Tracker

	clientConnector             tfClient.TfProxyConnectorI[*v1alpha1.ServiceInstance]
	newServicePlanInitializerFn func() Initializer
	resourcetracker             tracking.ReferenceResolverTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	_, ok := mg.(*v1alpha1.ServiceInstance)
	if !ok {
		return nil, errors.New(errNotServiceInstance)
	}
	if err := c.resourcetracker.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackRUsage)
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

	return &external{tfClient: client, kube: c.kube, tracker: c.resourcetracker}, nil
}

type external struct {
	tfClient tfClient.TfProxyControllerI
	kube     client.Client
	tracker  tracking.ReferenceResolverTracker
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

	// ADR: Check if there's a conflict error from previous Create attempt
	// AND external-name is not set (meaning user didn't intend to adopt)
	// TODO: what if the user changes the specs? Then it will stay in crash loop that is in wanted
	if meta.GetExternalName(cr) == "" {
		// Check if the LastAsyncOperation has a conflict error
		lastAsyncCond := cr.GetCondition(ujresource.TypeLastAsyncOperation)
		if lastAsyncCond.Message != "" && strings.Contains(lastAsyncCond.Message, "Conflict") {
			// ADR: Resource already exists but user hasn't set external-name
			// Return ResourceExists: false to stay in error loop
			// This forces user to set external-name to adopt the resource
			return managed.ExternalObservation{
				ResourceExists: false,
			}, errors.New("creation failed - resource already exists. Please set external-name annotation to adopt the existing resource")
		}
	}

	if meta.GetExternalName(cr) != "" {
		if !isValidUUID(meta.GetExternalName(cr)) {
			return managed.ExternalObservation{}, errors.New("external-name is not a valid UUID. Please check the value of the external-name annotation")
		}
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
			// Only set Available condition if ManagementPolicy is not only "Observe"
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

	// ADR: User has added external-name annotation with the external resource identifier
	// If external-name is set, this indicates an existing resource to be imported/adopted
	if meta.GetExternalName(cr) != "" {
		return managed.ExternalCreation{}, errors.New("cannot create: external-name already set. This resource appears to be managed/imported.")
	}

	// ADR: setting external-name not possible due to an async operation
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
		// If err is 404 not found, safely ignore as the resource is already deleted.
		if isNotFound(err) {
			return managed.ExternalDelete{}, nil
		}
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteInstance)
	}
	return managed.ExternalDelete{}, nil
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

func isObserveOnly(cr *v1alpha1.ServiceInstance) bool {
	policies := cr.GetManagementPolicies()
	if len(policies) == 0 {
		return false
	}
	// Check if the only policy is "Observe"
	if len(policies) == 1 && string(policies[0]) == "Observe" {
		return true
	}
	return false
}

func isNotFound(err error) bool {
	return false // TODO: implement proper not found error check based on the error type returned by the tf client
}

func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
