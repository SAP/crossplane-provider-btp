package servicebinding

import (
	"context"
	"math/rand"
	"strings"
	"sync"
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
	errNotServiceBinding = "managed resource is not a ServiceBinding custom resource"
	errTrackPCUsage      = "cannot track ProviderConfig usage"
	errGetPC             = "cannot get ProviderConfig"
	errGetCreds          = "cannot get credentials"

	errObserveBinding = "cannot observe servicebinding"
	errCreateBinding  = "cannot create servicebinding"
	errSaveData       = "cannot update cr data"
	errGetBinding     = "cannot get servicebinding"
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

	return errors.Wrap(uErr, errSaveData)
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

	// when working with tf proxy resources we want to keep the Connect() logic as part of the delgating Connect calls of the native resources to
	// deal with errors in the part of process that they belong to
	client, err := c.clientConnector.Connect(ctx, mg.(*v1alpha1.ServiceBinding))
	if err != nil {
		return nil, err
	}

	ext := &external{
		tfClient: client,
		kube:     c.kube,
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
	tfClient    tfClient.TfProxyControllerI
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

	// Determine instance name and UID (use stored values or fall back to CR values for old instances)
	instanceName := cr.Status.AtProvider.InstanceName
	instanceUID := cr.Status.AtProvider.InstanceUID
	if instanceName == "" {
		instanceName = cr.Name
	}
	if instanceUID == "" {
		instanceUID = string(cr.UID)
	}

	observation, tfResource, err := e.observeInstance(ctx, cr, instanceName, types.UID(instanceUID), cr.Status.AtProvider.ID)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetBinding)
	}

	// Only check rotation if resource exists
	if observation.ResourceExists {

		// Extract and update data from TF resource if available and up-to-date
		if observation.ResourceUpToDate && tfResource != nil && internal.Val(tfResource.Status.AtProvider.State) == "succeeded" {
			if err := e.updateServiceBindingFromTfResource(ctx, cr, tfResource); err != nil {
				return managed.ExternalObservation{}, errors.Wrap(err, "cannot update AtProvider data from TF resource")
			} else {
				if err := e.kube.Status().Update(ctx, cr); err != nil {
					return managed.ExternalObservation{}, errors.Wrap(err, "cannot update status")
				}
			}
		}

		// Update ResourceUpToDate based on expired keys
		observation.ResourceUpToDate = observation.ResourceUpToDate && !e.keyRotator.HasExpiredKeys(cr)

		// Check if binding should be retired for rotation
		if e.keyRotator.RetireBinding(cr) {
			if err := e.kube.Status().Update(ctx, cr); err != nil {
				return managed.ExternalObservation{}, errors.Wrap(err, "cannot update status after retiring binding")
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

	// Use our createInstance function to create a new binding with random name/UID
	instanceName, instanceUID, creation, err := e.createInstance(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBinding)
	}

	// Store the generated instance info in the CR status for tracking
	cr.Status.AtProvider.InstanceName = instanceName
	cr.Status.AtProvider.InstanceUID = string(instanceUID)

	// Explicitly persist the status changes immediately
	if err := e.kube.Status().Update(ctx, cr); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot update status with instance tracking info")
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
		update, err := e.updateInstance(ctx, cr, cr.Status.AtProvider.InstanceName, types.UID(cr.Status.AtProvider.InstanceUID), cr.Status.AtProvider.ID)
		if err != nil {
			return managed.ExternalUpdate{}, err
		}
		updateResult = update
	}

	// Clean up expired keys if there are any retired keys
	if cr.Status.AtProvider.RetiredKeys != nil {
		if newRetiredKeys, err := e.keyRotator.DeleteExpiredKeys(ctx, cr); err != nil {
			return managed.ExternalUpdate{}, errors.Wrap(err, "cannot delete expired keys")
		} else {
			cr.Status.AtProvider.RetiredKeys = newRetiredKeys
			// Explicitly persist the updated retired keys list immediately
			if err := e.kube.Status().Update(ctx, cr); err != nil {
				return managed.ExternalUpdate{}, errors.Wrap(err, "cannot update status with retired keys changes")
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

	// Clean up all retired keys before deleting the main binding
	if err := e.keyRotator.DeleteRetiredKeys(ctx, cr); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete retired keys")
	}

	if err := e.deleteInstance(ctx, cr, cr.Status.AtProvider.InstanceName, types.UID(cr.Status.AtProvider.InstanceUID), cr.Status.AtProvider.ID); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete servicebinding")
	}
	return managed.ExternalDelete{}, nil
}

// createInstance creates a new service binding instance with randomly generated name and UID
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client create command
// Returns the generated instance name, UID, and ExternalCreation for tracking
func (e *external) createInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding) (string, types.UID, managed.ExternalCreation, error) {
	// Generate random name and UID for the instance
	instanceName := randomName(publicCR.Spec.ForProvider.Name)
	instanceUID := randomUID()

	// Create a SubaccountServiceBinding resource with the random name and UID
	subaccountBinding := e.buildSubaccountServiceBinding(publicCR, instanceName, instanceUID, "")

	// Connect using the stored connector
	tfController, err := e.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return "", "", managed.ExternalCreation{}, errors.Wrap(err, "failed to connect TF controller")
	}

	// Create the TF resource using the TF client
	creation, err := tfController.Create(ctx, subaccountBinding)
	if err != nil {
		return "", "", managed.ExternalCreation{}, errors.Wrap(err, "failed to create TF resource")
	}

	return instanceName, instanceUID, creation, nil
}

// deleteInstance deletes a service binding instance with a different name and UID
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client delete command
func (e *external) deleteInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding, targetName string, targetUID types.UID, targetExternalName string) error {
	// Create a SubaccountServiceBinding resource with the target name and UID
	subaccountBinding := e.buildSubaccountServiceBinding(publicCR, targetName, targetUID, targetExternalName)
	subaccountBinding.SetDeletionTimestamp(internal.Ptr(metav1.NewTime(time.Now())))

	// Set the deleting condition on the SubaccountServiceBinding
	subaccountBinding.SetConditions(xpv1.Deleting())

	// Connect using the stored connector
	tfController, err := e.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return errors.Wrap(err, "failed to connect TF controller")
	}

	// Delete the TF resource using the TF client
	if _, err := tfController.Delete(ctx, subaccountBinding); err != nil {
		return errors.Wrap(err, "failed to delete TF resource")
	}

	return nil
}

// updateInstance updates a service binding instance with a different name and UID
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client update command
func (e *external) updateInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding, targetName string, targetUID types.UID, targetExternalName string) (managed.ExternalUpdate, error) {
	// Create a SubaccountServiceBinding resource with the target name and UID
	subaccountBinding := e.buildSubaccountServiceBinding(publicCR, targetName, targetUID, targetExternalName)

	// Connect using the stored connector
	tfController, err := e.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "failed to connect TF controller")
	}

	// Update the TF resource using the TF client
	return tfController.Update(ctx, subaccountBinding)
}

// observeInstance observes a service binding instance with a different name and UID
// by mapping the public CR to a TF CR, overwriting name and UID, and calling the TF client observe command
// Returns the observation and the TF resource for data extraction
func (e *external) observeInstance(ctx context.Context, publicCR *v1alpha1.ServiceBinding, targetName string, targetUID types.UID, targetExternalName string) (managed.ExternalObservation, *v1alpha1.SubaccountServiceBinding, error) {
	// Create a SubaccountServiceBinding resource with the target name and UID
	subaccountBinding := e.buildSubaccountServiceBinding(publicCR, targetName, targetUID, targetExternalName)

	// Connect using the stored connector
	tfController, err := e.sbConnector.Connect(ctx, subaccountBinding)
	if err != nil {
		return managed.ExternalObservation{}, nil, errors.Wrap(err, "failed to connect TF controller")
	}

	// Observe the TF resource using the TF client
	observation, err := tfController.Observe(ctx, subaccountBinding)
	if err != nil {
		return managed.ExternalObservation{}, nil, errors.Wrap(err, "failed to observe TF resource")
	}

	// If this is the current instance (not a retired one), set available condition
	if observation.ResourceExists && observation.ResourceUpToDate &&
		(targetName == publicCR.Status.AtProvider.InstanceName ||
			(publicCR.Status.AtProvider.InstanceName == "" && targetName == publicCR.Name)) {

		publicCR.SetConditions(xpv1.Available())
	}

	return observation, subaccountBinding, nil
}

const letterBytes = "abcdefghijklmnopqrstuvwxyz1234567890"

const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var (
	src      = rand.NewSource(time.Now().UnixNano())
	srcMutex sync.Mutex
)

func randomString(n int) string {
	sb := strings.Builder{}
	sb.Grow(n)

	srcMutex.Lock()
	defer srcMutex.Unlock()

	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			sb.WriteByte(letterBytes[idx])
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return sb.String()
}

func randomName(name string) string {
	if len(name) > 0 && name[len(name)-1] == '-' {
		name = name[:len(name)-1]
	}
	newName := name + "-" + randomString(5)
	return newName
}

func randomUID() types.UID {
	return types.UID(randomString(8))
}

// updateServiceBindingFromTfResource extracts data from SubaccountServiceBinding and updates the public ServiceBinding CR
func (e *external) updateServiceBindingFromTfResource(ctx context.Context, publicCR *v1alpha1.ServiceBinding, tfResource *v1alpha1.SubaccountServiceBinding) error {
	// Update external name if different
	tfExternalName := meta.GetExternalName(tfResource)
	if meta.GetExternalName(publicCR) != tfExternalName {
		meta.SetExternalName(publicCR, tfExternalName)
	}

	// Extract and update AtProvider data from TF resource status
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
