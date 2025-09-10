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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
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

	clientConnector tfClient.TfProxyConnectorI[*v1alpha1.ServiceBinding]
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	_, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return nil, errors.New(errNotServiceBinding)
	}

	ext := &external{
		kube: c.kube,
		sbConnector: tfClient.NewInternalTfConnector(
			c.kube,
			"btp_subaccount_service_binding",
			v1alpha1.SubaccountServiceBinding_GroupVersionKind,
			false,
			nil,
		),
	}
	ext.keyRotator = &SBKeyRotator{external: ext}
	return ext, nil
}

type external struct {
	kube        client.Client
	keyRotator  KeyRotator
	sbConnector managed.ExternalConnecter
}

// Disconnect is a no-op for the external client to close its connection.
// Since we dont need this, we only have it to fullfil the interface.
func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotServiceBinding)
	}

	// Determine instance name (use stored value or fall back to CR name for old instances)
	instanceName := cr.Status.AtProvider.InstanceName
	if instanceName == "" {
		instanceName = cr.Name
	}

	observation, tfResource, err := e.observeInstance(ctx, cr, instanceName, cr.Status.AtProvider.ID)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetBinding)
	}

	if observation.ResourceExists {

		// Extract and update data from TF resource if available and up-to-date
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

	instanceName, _, creation, err := e.createInstance(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBinding)
	}

	cr.Status.AtProvider.InstanceName = instanceName

	if err := e.kube.Status().Update(ctx, cr); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errUpdateStatus)
	}

	// Remove force rotation annotation after successful creation
	if cr.ObjectMeta.Annotations != nil {
		if _, ok := cr.ObjectMeta.Annotations[ForceRotationKey]; ok {
			meta.RemoveAnnotations(cr, ForceRotationKey)
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
		instanceName := cr.Status.AtProvider.InstanceName
		if instanceName == "" {
			instanceName = cr.Name
		}

		update, err := e.updateInstance(ctx, cr, instanceName, cr.Status.AtProvider.ID)
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

	instanceName := cr.Status.AtProvider.InstanceName
	if instanceName == "" {
		instanceName = cr.Name
	}

	if err := e.deleteInstance(ctx, cr, instanceName, cr.Status.AtProvider.ID); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteServiceBinding)
	}
	return managed.ExternalDelete{}, nil
}

// isRotationEnabled checks if rotation is currently enabled for the service binding
func (e *external) isRotationEnabled(cr *v1alpha1.ServiceBinding) bool {
	// Check for force rotation annotation
	if cr.ObjectMeta.Annotations != nil {
		if _, ok := cr.ObjectMeta.Annotations[ForceRotationKey]; ok {
			return true
		}
	}

	// Check if rotation frequency is configured
	if cr.Spec.ForProvider.Rotation != nil && cr.Spec.ForProvider.Rotation.Frequency != nil {
		return true
	}

	return false
}

// createInstance creates a new service binding instance with conditional name generation
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client create command
// Returns the generated instance name, UID, and ExternalCreation for tracking
func (e *external) createInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding) (string, types.UID, managed.ExternalCreation, error) {
	var instanceName string

	hasRetiredKeys := len(publicCR.Status.AtProvider.RetiredKeys) > 0

	if e.isRotationEnabled(publicCR) || hasRetiredKeys {
		// Use random suffix in these cases:
		// 1. Rotation is currently enabled
		// 2. There are retired keys (rotation was enabled before -> avoid conflicts)
		instanceName = randomName(publicCR.Spec.ForProvider.Name)
	} else {
		// No rotation enabled and no history of rotation - use original name without suffix
		instanceName = publicCR.Spec.ForProvider.Name
	}

	instanceUID := generateInstanceUID(publicCR.UID, instanceName)

	subaccountBinding := e.buildSubaccountServiceBinding(publicCR, instanceName, instanceUID, "")

	tfController, err := e.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return "", "", managed.ExternalCreation{}, errors.Wrap(err, errConnectTfController)
	}

	creation, err := tfController.Create(ctx, subaccountBinding)
	if err != nil {
		return "", "", managed.ExternalCreation{}, errors.Wrap(err, errCreateTfResource)
	}

	return instanceName, instanceUID, creation, nil
}

// deleteInstance deletes a service binding instance with a different name and UID
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client delete command
func (e *external) deleteInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding, targetName string, targetExternalName string) error {
	targetUID := generateInstanceUID(publicCR.UID, targetName)
	subaccountBinding := e.buildSubaccountServiceBinding(publicCR, targetName, targetUID, targetExternalName)

	subaccountBinding.SetDeletionTimestamp(internal.Ptr(metav1.NewTime(time.Now())))
	subaccountBinding.SetConditions(xpv1.Deleting())

	tfController, err := e.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return errors.Wrap(err, errConnectTfController)
	}

	if _, err := tfController.Delete(ctx, subaccountBinding); err != nil {
		return errors.Wrap(err, errDeleteTfResource)
	}

	return nil
}

// updateInstance updates a service binding instance with a different name and UID
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client update command
func (e *external) updateInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding, targetName string, targetExternalName string) (managed.ExternalUpdate, error) {
	targetUID := generateInstanceUID(publicCR.UID, targetName)
	subaccountBinding := e.buildSubaccountServiceBinding(publicCR, targetName, targetUID, targetExternalName)

	tfController, err := e.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errConnectTfController)
	}

	return tfController.Update(ctx, subaccountBinding)
}

// observeInstance observes a service binding instance with a different name and UID
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client observe command
// Returns the observation and the TF resource for data extraction
func (e *external) observeInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding, targetName string, targetExternalName string) (managed.ExternalObservation, *v1alpha1.SubaccountServiceBinding, error) {
	targetUID := generateInstanceUID(publicCR.UID, targetName)
	subaccountBinding := e.buildSubaccountServiceBinding(publicCR, targetName, targetUID, targetExternalName)

	tfController, err := e.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return managed.ExternalObservation{}, nil, errors.Wrap(err, errConnectTfController)
	}

	observation, err := tfController.Observe(ctx, subaccountBinding)
	if err != nil {
		return managed.ExternalObservation{}, nil, errors.Wrap(err, errObserveTfResource)
	}

	if observation.ResourceExists && observation.ResourceUpToDate &&
		(targetName == publicCR.Status.AtProvider.InstanceName ||
			(publicCR.Status.AtProvider.InstanceName == "" && targetName == publicCR.Name)) {

		publicCR.SetConditions(xpv1.Available())
	}

	return observation, subaccountBinding, nil
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

// buildSubaccountServiceBinding creates a SubaccountServiceBinding resource from a ServiceBinding
func (e *external) buildSubaccountServiceBinding(sb *v1alpha1.ServiceBinding, name string, uid types.UID, externalName string) *v1alpha1.SubaccountServiceBinding {
	sBinding := &v1alpha1.SubaccountServiceBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.SubaccountServiceBinding_Kind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			UID:               uid,
			DeletionTimestamp: sb.DeletionTimestamp,
		},
		Spec: v1alpha1.SubaccountServiceBindingSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: sb.GetProviderConfigReference().Name,
				},
				ManagementPolicies: []xpv1.ManagementAction{xpv1.ManagementActionAll},
			},
			ForProvider: v1alpha1.SubaccountServiceBindingParameters{
				SubaccountID:      sb.Spec.ForProvider.SubaccountID,
				ServiceInstanceID: sb.Spec.ForProvider.ServiceInstanceID,
				Name:              &name,
			},
		},
		Status: v1alpha1.SubaccountServiceBindingStatus{},
	}

	if externalName != "" {
		meta.SetExternalName(sBinding, externalName)
	}
	return sBinding
}
