package servicebinding

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	servicebindingclient "github.com/sap/crossplane-provider-btp/internal/clients/account/servicebinding"
	tfClient "github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
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
	errConnectTfController  = "failed to connect TF controller"
	errCreateTfResource     = "failed to create TF resource"
	errDeleteTfResource     = "failed to delete TF resource"
	errObserveTfResource    = "failed to observe TF resource"
)

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
	kube  client.Client
	usage resource.Tracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	_, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return nil, errors.New(errNotServiceBinding)
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
	}

	// Create key rotator with the external client as instance deleter
	ext.keyRotator = servicebindingclient.NewSBKeyRotator(ext)

	return ext, nil
}

type external struct {
	kube            client.Client
	keyRotator      servicebindingclient.KeyRotator
	instanceManager *servicebindingclient.InstanceManager
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

	// Use btpName from spec, fall back to name for backward compatibility
	var btpName string
	if cr.Spec.BtpName != nil {
		btpName = *cr.Spec.BtpName
	} else {
		// Backward compatibility: use the original name
		btpName = cr.Spec.ForProvider.Name
	}

	observation, tfResource, err := e.instanceManager.ObserveInstance(ctx, cr, btpName, cr.Status.AtProvider.ID)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetBinding)
	}

	// Extract and update data from TF resource if available and up-to-date
	if observation.ResourceExists {
		if observation.ResourceUpToDate && tfResource != nil && internal.Val(tfResource.Status.AtProvider.State) == "succeeded" {
			if err := e.updateServiceBindingFromTfResource(cr, tfResource); err != nil {
				return managed.ExternalObservation{}, errors.Wrap(err, errObserveSaveBinding)
			} else {
				if err := e.kube.Status().Update(ctx, cr); err != nil {
					return managed.ExternalObservation{}, errors.Wrap(err, errUpdateStatus)
				}
			}
		}

		observation.ResourceUpToDate = observation.ResourceUpToDate && !e.keyRotator.HasExpiredKeys(cr)

		// Retire binding conditionally
		if e.keyRotator.RetireBinding(cr) {
			if err := e.kube.Status().Update(ctx, cr); err != nil {
				return managed.ExternalObservation{}, errors.Wrap(err, errUpdateStatus)
			}
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
	}

	return observation, nil
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

	// Update the spec with the generated btpName
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

	// Remove force rotation annotation after successful creation
	if cr.ObjectMeta.Annotations != nil {
		if _, ok := cr.ObjectMeta.Annotations[servicebindingclient.ForceRotationKey]; ok {
			meta.RemoveAnnotations(cr, servicebindingclient.ForceRotationKey)
		}
	}

	return creation, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotServiceBinding)
	}

	// Check if current binding is already retired - if so, skip update as service bindings are immutable
	currentBindingRetired := false
	for _, retiredKey := range cr.Status.AtProvider.RetiredKeys {
		if retiredKey.ID == cr.Status.AtProvider.ID {
			currentBindingRetired = true
			break
		}
	}

	// Only update if the current binding is not retired (service bindings are immutable in BTP)
	var updateResult managed.ExternalUpdate
	if !currentBindingRetired {
		// Use btpName from spec, fall back to name for backward compatibility
		var btpName string
		if cr.Spec.BtpName != nil {
			btpName = *cr.Spec.BtpName
		} else {
			btpName = cr.Spec.ForProvider.Name
		}

		update, err := e.instanceManager.UpdateInstance(ctx, cr, btpName, cr.Status.AtProvider.ID)
		if err != nil {
			return managed.ExternalUpdate{}, err
		}
		updateResult = update
	}

	// Clean up expired keys if there are any retired keys
	if cr.Status.AtProvider.RetiredKeys != nil {
		if newRetiredKeys, err := e.keyRotator.DeleteExpiredKeys(ctx, cr); err != nil {
			return managed.ExternalUpdate{}, errors.Wrap(err, errDeleteExpiredKeys)
		} else {
			cr.Status.AtProvider.RetiredKeys = newRetiredKeys
			if err := e.kube.Status().Update(ctx, cr); err != nil {
				return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateStatus)
			}
		}
	}

	return updateResult, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotServiceBinding)
	}
	cr.SetConditions(xpv1.Deleting())

	if err := e.keyRotator.DeleteRetiredKeys(ctx, cr); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteRetiredKeys)
	}

	// Use btpName from spec, fall back to name for backward compatibility
	var btpName string
	if cr.Spec.BtpName != nil {
		btpName = *cr.Spec.BtpName
	} else {
		btpName = cr.Spec.ForProvider.Name
	}

	if err := e.instanceManager.DeleteInstance(ctx, cr, btpName, cr.Status.AtProvider.ID); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteServiceBinding)
	}
	return managed.ExternalDelete{}, nil
}

// isRotationEnabled checks if rotation is currently enabled for the service binding
func (e *external) isRotationEnabled(cr *v1alpha1.ServiceBinding) bool {
	// Check for force rotation annotation
	if cr.ObjectMeta.Annotations != nil {
		if _, ok := cr.ObjectMeta.Annotations[servicebindingclient.ForceRotationKey]; ok {
			return true
		}
	}

	// Check if rotation frequency is configured
	if cr.Spec.ForProvider.Rotation != nil && cr.Spec.ForProvider.Rotation.Frequency != nil {
		return true
	}

	return false
}

// updateServiceBindingFromTfResource extracts data from SubaccountServiceBinding and updates the public ServiceBinding CR
func (e *external) updateServiceBindingFromTfResource(publicCR *v1alpha1.ServiceBinding, tfResource *v1alpha1.SubaccountServiceBinding) error {
	meta.SetExternalName(publicCR, meta.GetExternalName(tfResource))

	publicCR.Status.AtProvider.ID = internal.Val(tfResource.Status.AtProvider.ID)
	publicCR.Status.AtProvider.Name = internal.Val(tfResource.Status.AtProvider.Name)
	publicCR.Status.AtProvider.Ready = tfResource.Status.AtProvider.Ready
	publicCR.Status.AtProvider.State = tfResource.Status.AtProvider.State
	publicCR.Status.AtProvider.CreatedDate = tfResource.Status.AtProvider.CreatedDate
	publicCR.Status.AtProvider.LastModified = tfResource.Status.AtProvider.LastModified
	publicCR.Status.AtProvider.Parameters = tfResource.Status.AtProvider.Parameters

	return nil
}
