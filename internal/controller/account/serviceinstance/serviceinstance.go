package serviceinstance

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ujresource "github.com/crossplane/upjet/pkg/resource"

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

	// ADR(external-name): Check if there's a conflict error from previous Create attempt
	// AND external-name is not set (meaning user didn't intend to adopt)
	// TODO: what if the user changes the specs? Then it will stay in crash loop that is in wanted
	if meta.GetExternalName(cr) == "" {
		// Check if the LastAsyncOperation has a conflict error
		lastAsyncCond := cr.GetCondition(ujresource.TypeLastAsyncOperation)
		if lastAsyncCond.Message != "" && strings.Contains(lastAsyncCond.Message, "Conflict") {
			// ADR(external-name): Resource already exists but user hasn't set external-name
			// Return ResourceExists: false to stay in error loop
			// This forces user to set external-name to adopt the resource
			return managed.ExternalObservation{
				ResourceExists: false,
			}, errors.New("creation failed - resource already exists. Please set external-name annotation to adopt the existing resource")
		}
	}

	if meta.GetExternalName(cr) != "" {
		if !internal.IsValidUUID(meta.GetExternalName(cr)) {
			return managed.ExternalObservation{}, errors.New("external-name is not a valid UUID. Please check the value of the external-name annotation and set it to the ServiceInstance ID (UUID format) if you want to adopt an existing resource, or remove the annotation if you want to create a new one")
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
	cr.Status.AtProvider.DashboardURL = sid.DashboardURL
	return nil
}

func isObserveOnly(cr *v1alpha1.ServiceInstance) bool {
	policies := cr.GetManagementPolicies()
	return len(policies) == 1 && policies[0] == xpv1.ManagementActionObserve
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "404")
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
