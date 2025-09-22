package servicebinding

import (
	"context"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	servicebindingclient "github.com/sap/crossplane-provider-btp/internal/clients/account/servicebinding"
	tfClient "github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotServiceBinding    = "managed resource is not a ServiceBinding custom resource"
	errCreateBinding        = "cannot create servicebinding"
	errObserveSaveBinding   = "cannot save observed data"
	errUpdateStatus         = "cannot update status"
	errGetBinding           = "cannot get servicebinding"
	errDeleteExpiredKeys    = "cannot delete expired keys"
	errDeleteRetiredKeys    = "cannot delete retired keys"
	errDeleteServiceBinding = "cannot delete servicebinding"
)

const iso8601Date = "2006-01-02T15:04:05Z0700"

// SaveConditionsFn Callback for persisting conditions in the CR
var saveCallback tfClient.SaveConditionsFn = func(ctx context.Context, kube client.Client, name string, conditions ...xpv1.Condition) error {

	si := &v1alpha1.ServiceBinding{}

	nn := types.NamespacedName{Name: name}
	if kErr := kube.Get(ctx, nn, si); kErr != nil {
		return errors.Wrap(kErr, errGetBinding)
	}

	si.SetConditions(conditions...)

	uErr := kube.Status().Update(ctx, si)

	return errors.Wrap(uErr, errObserveSaveBinding)
}

type connector struct {
	kube            client.Client
	usage           resource.Tracker
	resourcetracker tracking.ReferenceResolverTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return nil, errors.New(errNotServiceBinding)
	}

	// Track resource references for dependency management
	if err := c.resourcetracker.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, "cannot track resource references")
	}

	sbConnector := tfClient.NewInternalTfConnector(
		c.kube,
		"btp_subaccount_service_binding",
		v1alpha1.SubaccountServiceBinding_GroupVersionKind,
		false,
		nil,
	)

	instanceManager := servicebindingclient.NewInstanceManager(sbConnector)

	ext := &external{
		kube:            c.kube,
		instanceManager: instanceManager,
		tracker:         c.resourcetracker,
	}

	// Create key rotator with the external client as instance deleter
	ext.keyRotator = servicebindingclient.NewSBKeyRotator(ext)

	return ext, nil
}

type external struct {
	kube            client.Client
	keyRotator      servicebindingclient.KeyRotator
	instanceManager *servicebindingclient.InstanceManager
	tracker         tracking.ReferenceResolverTracker
}

// Disconnect is a no-op for the external client to close its connection.
// Since we dont need this, we only have it to fullfil the interface.
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

// DeleteInstance implements the InstanceDeleter interface for the key rotator
func (e *external) DeleteInstance(ctx context.Context, cr *v1alpha1.ServiceBinding, targetName string, targetExternalName string) error {
	return e.instanceManager.DeleteInstance(ctx, cr, targetName, targetExternalName)
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotServiceBinding)
	}

	btpName := getBtpName(cr)

	observation, tfResource, err := e.instanceManager.ObserveInstance(ctx, cr, btpName, cr.Status.AtProvider.ID)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetBinding)
	}

	// Extract and update data from TF resource if available and up-to-date
	if !observation.ResourceExists {
		return observation, nil
	}

	if observation.ResourceUpToDate && tfResource != nil && internal.Val(tfResource.Status.AtProvider.State) == "succeeded" {
		if err := e.updateServiceBindingFromTfResource(cr, tfResource); err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errObserveSaveBinding)
		}

		if err := e.kube.Status().Update(ctx, cr); err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errUpdateStatus)

		}
	}

	observation.ResourceUpToDate = observation.ResourceUpToDate && !e.keyRotator.HasExpiredKeys(cr)

	// Retire binding conditionally
	if !e.keyRotator.RetireBinding(cr) {
		return observation, nil
	}

	if err := e.kube.Status().Update(ctx, cr); err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errUpdateStatus)
	}
	return managed.ExternalObservation{ResourceExists: false}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotServiceBinding)
	}

	cr.SetConditions(xpv1.Creating())

	// Generate btpName if not already set in spec
	var btpName string
	if e.isRotationEnabled(cr) {
		btpName = servicebindingclient.GenerateRandomName(cr.Spec.ForProvider.Name)
	} else {
		btpName = cr.Spec.ForProvider.Name
	}
	cr.Spec.BtpName = &btpName

	if err := e.kube.Update(ctx, cr); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errUpdateStatus)
	}

	_, _, creation, err := e.instanceManager.CreateInstance(ctx, cr, *cr.Spec.BtpName)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBinding)
	}

	if err := e.kube.Status().Update(ctx, cr); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errUpdateStatus)
	}

	meta.RemoveAnnotations(cr, servicebindingclient.ForceRotationKey)

	return creation, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotServiceBinding)
	}

	// Only update if the current binding is not retired (service bindings are immutable in BTP)
	updateResult := managed.ExternalUpdate{}
	if !e.keyRotator.IsCurrentBindingRetired(cr) {
		btpName := getBtpName(cr)

		update, err := e.instanceManager.UpdateInstance(ctx, cr, btpName, cr.Status.AtProvider.ID)
		if err != nil {
			return managed.ExternalUpdate{}, err
		}
		updateResult = update
	}

	// Clean up expired keys if there are any retired keys
	newRetiredKeys, err := e.keyRotator.DeleteExpiredKeys(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errDeleteExpiredKeys)
	}

	cr.Status.AtProvider.RetiredKeys = newRetiredKeys

	if err := e.kube.Status().Update(ctx, cr); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateStatus)
	}

	return updateResult, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotServiceBinding)
	}
	cr.SetConditions(xpv1.Deleting())

	// Set resource usage conditions to check dependencies
	e.tracker.SetConditions(ctx, cr)

	// Block deletion if other resources are still using this ServiceBinding
	if blocked := e.tracker.DeleteShouldBeBlocked(mg); blocked {
		return managed.ExternalDelete{}, errors.New(providerv1alpha1.ErrResourceInUse)
	}

	if err := e.keyRotator.DeleteRetiredKeys(ctx, cr); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteRetiredKeys)
	}

	btpName := getBtpName(cr)

	if err := e.instanceManager.DeleteInstance(ctx, cr, btpName, cr.Status.AtProvider.ID); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteServiceBinding)
	}

	return managed.ExternalDelete{}, nil
}

// isRotationEnabled checks if rotation is currently enabled for the service binding
func (e *external) isRotationEnabled(cr *v1alpha1.ServiceBinding) bool {
	if metav1.HasAnnotation(cr.ObjectMeta, servicebindingclient.ForceRotationKey) {
		return true
	}

	if cr.Spec.ForProvider.Rotation != nil {
		return true
	}

	return false
}

// getBtpName returns the btpName from spec, falling back to name for backward compatibility
func getBtpName(cr *v1alpha1.ServiceBinding) string {
	if cr.Spec.BtpName != nil {
		return *cr.Spec.BtpName
	}
	return cr.Spec.ForProvider.Name
}

// updateServiceBindingFromTfResource extracts data from SubaccountServiceBinding and updates the public ServiceBinding CR
func (e *external) updateServiceBindingFromTfResource(publicCR *v1alpha1.ServiceBinding, tfResource *v1alpha1.SubaccountServiceBinding) error {
	meta.SetExternalName(publicCR, meta.GetExternalName(tfResource))

	var createdDate *v1.Time = nil
	if tfResource.Status.AtProvider.CreatedDate != nil {
		// The date is in the iso8601 format, which is not the same as the RFC3339 format the parameter claims to have
		cd, err := parseIso8601Date(*tfResource.Status.AtProvider.CreatedDate)
		if err != nil {
			return err
		}

		createdDate = &cd
	}
	var lastModified *v1.Time = nil
	if tfResource.Status.AtProvider.LastModified != nil {
		// The date is in the iso8601 format, which is not the same as the RFC3339 format the parameter claims to have
		lm, err := parseIso8601Date(*tfResource.Status.AtProvider.LastModified)
		if err != nil {
			return err
		}

		lastModified = &lm
	}

	publicCR.Status.AtProvider.ID = internal.Val(tfResource.Status.AtProvider.ID)
	publicCR.Status.AtProvider.Name = internal.Val(tfResource.Status.AtProvider.Name)
	publicCR.Status.AtProvider.Ready = tfResource.Status.AtProvider.Ready
	publicCR.Status.AtProvider.State = tfResource.Status.AtProvider.State
	publicCR.Status.AtProvider.CreatedDate = createdDate
	publicCR.Status.AtProvider.LastModified = lastModified
	publicCR.Status.AtProvider.Parameters = tfResource.Status.AtProvider.Parameters

	return nil
}

func parseIso8601Date(t string) (v1.Time, error) {
	iTime, err := time.Parse(iso8601Date, t)
	if err != nil {
		return v1.Time{}, err
	}

	return v1.Time{
		Time: iTime,
	}, nil
}
