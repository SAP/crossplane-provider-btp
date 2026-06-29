package servicebinding

import (
	"context"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	servicebindingclient "github.com/sap/crossplane-provider-btp/internal/clients/account/servicebinding"
	"github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	tfClient "github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/reconcilerutil"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	errNotServiceBinding    = "managed resource is not a ServiceBinding custom resource"
	errCreateBinding        = "cannot create servicebinding"
	errUpdateStatus         = "cannot update status"
	errGetBinding           = "cannot get servicebinding"
	errDeleteExpiredKeys    = "cannot delete expired keys"
	errDeleteRetiredKeys    = "cannot delete retired keys"
	errDeleteServiceBinding = "cannot delete servicebinding"
	errFlattenSecret        = "cannot flatten secret"
)

const iso8601Date = "2006-01-02T15:04:05Z0700"

var newTfConnectorFn = func(kube kubeclient.Client) servicebindingclient.TfConnector {
	return tfClient.NewInternalTfConnector(
		kube,
		"btp_subaccount_service_binding",
		v1alpha1.SubaccountServiceBinding_GroupVersionKind,
		false,
		nil,
	)
}

// ServiceBindingClientFactory creates ServiceBindingClient instances
type ServiceBindingClientFactory interface {
	CreateClient(ctx context.Context, cr *v1alpha1.ServiceBinding, targetName string, targetExternalName string) (servicebindingclient.ServiceBindingClientInterface, error)
}

// DefaultServiceBindingClientFactory is the production implementation
type DefaultServiceBindingClientFactory struct {
	kube        kubeclient.Client
	tfConnector servicebindingclient.TfConnector
}

func (f *DefaultServiceBindingClientFactory) CreateClient(ctx context.Context, cr *v1alpha1.ServiceBinding, targetName string, targetExternalName string) (servicebindingclient.ServiceBindingClientInterface, error) {
	client, err := servicebindingclient.NewServiceBindingClient(ctx, f.kube, f.tfConnector, cr, targetName, targetExternalName)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func newServiceBindingClientFactory(kube kubeclient.Client, tfConnector servicebindingclient.TfConnector) ServiceBindingClientFactory {
	return &DefaultServiceBindingClientFactory{
		kube:        kube,
		tfConnector: tfConnector,
	}
}

var newSBKeyRotatorFn = func(bindingDeleter servicebindingclient.BindingDeleter) servicebindingclient.KeyRotator {
	return servicebindingclient.NewSBKeyRotator(bindingDeleter)
}

type connector struct {
	kube              kubeclient.Client
	usage             providerconfig.LegacyTracker
	resourcetracker   tracking.ReferenceResolverTracker
	clientFactory     ServiceBindingClientFactory
	newSBKeyRotatorFn func(servicebindingclient.BindingDeleter) servicebindingclient.KeyRotator
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

	var targetName string
	if cr.Status.AtProvider.Name != "" {
		targetName = cr.Status.AtProvider.Name
	} else {
		targetName = cr.Spec.ForProvider.Name
	}

	client, err := c.clientFactory.CreateClient(ctx, cr, targetName, meta.GetExternalName(cr))
	if err != nil {
		return nil, errors.Wrap(err, "cannot create client")
	}

	ext := &external{
		kube:          c.kube,
		clientFactory: c.clientFactory,
		tracker:       c.resourcetracker,
		client:        client,
	}

	ext.keyRotator = c.newSBKeyRotatorFn(ext)

	return ext, nil
}

type external struct {
	kube          kubeclient.Client
	keyRotator    servicebindingclient.KeyRotator
	client        servicebindingclient.ServiceBindingClientInterface
	clientFactory ServiceBindingClientFactory
	tracker       tracking.ReferenceResolverTracker
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

	observation, tfResource, err := e.client.Observe(ctx)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetBinding)
	}

	// Extract and update data from TF resource if available and up-to-date
	if !observation.ResourceExists {
		// Upjet's workspace-state-based view says the binding doesn't exist, but
		// the workspace is per-pod ephemeral state (emptyDir). After a pod
		// restart this can mis-report a still-existing BTP binding as gone --
		// during a deletion that causes the finalizer to be dropped and the BTP
		// binding to be permanently leaked. Consult the durable source (the SAP
		// Service Manager API) before trusting NotExists.
		if adopted, recErr := e.recoverByBTPName(ctx, cr); recErr == nil && adopted {
			return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
		}
		return observation, nil
	}

	if observation.ResourceUpToDate && tfResource != nil {
		if err := reconcilerutil.UpdateStatusWithRetry(ctx, e.kube, cr, 3, func(cr *v1alpha1.ServiceBinding) error {
			return e.updateServiceBindingFromTfResource(cr, tfResource)
		}); err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errUpdateStatus)
		}
	}

	observation.ConnectionDetails, err = processConnectionDetails(cr, observation.ConnectionDetails)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errFlattenSecret)
	}

	if cr.Spec.SecretFormat == SecretFormatSAPKubernetes && observation.ConnectionDetails != nil {
		observation.ConnectionDetails, err = e.enrichWithSAPMetadata(ctx, cr, observation.ConnectionDetails)
		if err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, "cannot enrich connection details")
		}
	}

	observation.ResourceUpToDate = observation.ResourceUpToDate && !e.keyRotator.HasExpiredKeys(cr)

	// Validate rotation settings and set status condition
	e.keyRotator.ValidateRotationSettings(cr)

	// Retire binding conditionally
	if !e.keyRotator.NeedRetirement(cr) {
		if !cr.GetDeletionTimestamp().IsZero() {
			return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
		}
		return observation, nil
	}

	if err := reconcilerutil.UpdateStatusWithRetry(ctx, e.kube, cr, 5, func(cr *v1alpha1.ServiceBinding) error {
		e.keyRotator.RetireBinding(cr)
		return nil
	}); err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errUpdateStatus)
	}

	if !cr.GetDeletionTimestamp().IsZero() {
		return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
	}
	return managed.ExternalObservation{ResourceExists: false}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotServiceBinding)
	}

	cr.SetConditions(xpv1.Creating())

	// Generate name based on rotation settings (pure, testable business logic)
	name := e.generateName(cr)

	client, err := e.clientFactory.CreateClient(ctx, cr, name, name)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBinding)
	}

	e.client = client

	externalName, creation, err := client.Create(ctx)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBinding)
	}

	meta.SetExternalName(cr, externalName)
	meta.RemoveAnnotations(cr, servicebindingclient.ForceRotationKey)

	// Call the kube client to update the external-name and force-rotation annotations
	if err := e.kube.Update(ctx, cr); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBinding)
	}

	creation.ConnectionDetails, err = processConnectionDetails(cr, creation.ConnectionDetails)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errFlattenSecret)
	}

	if cr.Spec.SecretFormat == SecretFormatSAPKubernetes && creation.ConnectionDetails != nil {
		creation.ConnectionDetails, err = e.enrichWithSAPMetadata(ctx, cr, creation.ConnectionDetails)
		if err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, "cannot enrich connection details")
		}
	}

	return creation, nil
}

// Update() does not make a real update of the service binding, because service
// bindings are immutable anyway. This behaviour is also disabled in the
// underlying terraform provider.
// Instead, Update() is only used to delete expired keys.
func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.ServiceBinding)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotServiceBinding)
	}

	// Clean up expired keys if there are any retired keys
	newRetiredKeys, deleteErr := e.keyRotator.DeleteExpiredKeys(ctx, cr)

	// store the result in the status even if errors are returned,
	// to remove keys for those where deletion was successfull
	if err := reconcilerutil.UpdateStatusWithRetry(ctx, e.kube, cr, 3, func(cr *v1alpha1.ServiceBinding) error {
		cr.Status.RetiredKeys = newRetiredKeys
		return nil
	}); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateStatus)
	}
	if deleteErr != nil {
		return managed.ExternalUpdate{}, errors.Wrap(deleteErr, errDeleteExpiredKeys)
	}

	return managed.ExternalUpdate{}, nil
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

	deletion, err := e.client.Delete(ctx)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteServiceBinding)
	}

	return deletion, nil
}

// DeleteBinding implements the BindingDeleter interface for the key rotator
func (e *external) DeleteBinding(ctx context.Context, cr *v1alpha1.ServiceBinding, targetName string, targetExternalName string) error {
	// The deletion timestamp must be set before the Connect() function is called on the external client. This fixes (#425).
	// Otherwise it could in some cases end in an error stating that `prevent_destroy` is set to `true` on the resource.
	cr = cr.DeepCopy()
	cr.SetDeletionTimestamp(internal.Ptr(metav1.Now()))
	cr.SetConditions(xpv1.Deleting())

	// Create a client for the specific binding to delete
	client, err := e.clientFactory.CreateClient(ctx, cr, targetName, targetExternalName)
	if err != nil {
		return err
	}
	_, err = client.Delete(ctx)
	return err
}

// isRotationEnabled checks if rotation is currently enabled for the service binding
func (e *external) isRotationEnabled(cr *v1alpha1.ServiceBinding) bool {
	if metav1.HasAnnotation(cr.ObjectMeta, servicebindingclient.ForceRotationKey) {
		return true
	}

	if cr.Spec.Rotation != nil {
		return true
	}

	return false
}

// generateName generates the target name for the service binding based on rotation settings
func (e *external) generateName(cr *v1alpha1.ServiceBinding) string {
	if e.isRotationEnabled(cr) {
		return servicebindingclient.GenerateRandomName(cr.Spec.ForProvider.Name)
	}
	return cr.Spec.ForProvider.Name
}

// recoverByBTPName tries to recover a missing-from-upjet ServiceBinding by
// looking it up directly on the SAP Service Manager API by name + parent
// service instance ID. On success it writes the discovered BTP UUID into the
// public CR's external-name annotation and returns (true, nil).
//
// All error and "not found" cases return (false, nil) so the caller can fall
// through to reporting NotExists -- the goal is to be additive (recover when
// possible) without regressing the failure semantics of the existing path.
func (e *external) recoverByBTPName(ctx context.Context, cr *v1alpha1.ServiceBinding) (bool, error) {
	// We need both the parent SI's UUID and the binding's BTP-side name.
	siID := internal.Val(cr.Spec.ForProvider.ServiceInstanceID)
	if siID == "" {
		return false, nil
	}
	name := cr.Status.AtProvider.Name
	if name == "" {
		// Status hasn't been populated yet (rotation case, never observed UpToDate).
		// For non-rotation bindings the spec name matches the BTP-side name and is
		// safe to use; for rotation bindings without status we can't reconstruct
		// the random suffix and must give up.
		if e.isRotationEnabled(cr) {
			return false, nil
		}
		name = cr.Spec.ForProvider.Name
		if name == "" {
			return false, nil
		}
	}

	// The SB CR doesn't carry the ServiceManager secret reference directly --
	// it's only on the parent ServiceInstance CR. Resolve it via the binding's
	// ServiceInstanceRef.
	siRef := cr.Spec.ForProvider.ServiceInstanceRef
	if siRef == nil || siRef.Name == "" {
		return false, nil
	}
	si := &v1alpha1.ServiceInstance{}
	if err := e.kube.Get(ctx, kubeclient.ObjectKey{Name: siRef.Name}, si); err != nil {
		return false, nil
	}

	creds, err := servicemanager.LoadCredsFromSecret(ctx, e.kube,
		si.Spec.ForProvider.ServiceManagerSecretNamespace,
		si.Spec.ForProvider.ServiceManagerSecret)
	if err != nil {
		return false, nil
	}
	smClient, err := servicemanager.NewServiceManagerClient(ctx, creds)
	if err != nil {
		return false, nil
	}

	id, err := smClient.FindServiceBindingIDByName(ctx, siID, name)
	if err != nil || id == "" {
		return false, nil
	}

	meta.SetExternalName(cr, id)
	if err := e.kube.Update(ctx, cr); err != nil {
		return false, errors.Wrap(err, "cannot persist recovered external-name")
	}
	return true, nil
}

// updateServiceBindingFromTfResource extracts data from SubaccountServiceBinding and updates the public ServiceBinding CR
func (e *external) updateServiceBindingFromTfResource(publicCR *v1alpha1.ServiceBinding, tfResource *v1alpha1.SubaccountServiceBinding) error {
	meta.SetExternalName(publicCR, meta.GetExternalName(tfResource))

	var createdDate *metav1.Time = nil
	if tfResource.Status.AtProvider.CreatedDate != nil {
		// The date is in the iso8601 format, which is not the same as the RFC3339 format the parameter claims to have
		cd, err := parseIso8601Date(*tfResource.Status.AtProvider.CreatedDate)
		if err != nil {
			return err
		}

		createdDate = &cd
	}

	var lastModified *metav1.Time = nil
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

	if *tfResource.Status.AtProvider.State == "succeeded" {
		publicCR.SetConditions(xpv1.Available())
	}

	return nil
}

func parseIso8601Date(t string) (metav1.Time, error) {
	iTime, err := time.Parse(iso8601Date, t)
	if err != nil {
		return metav1.Time{}, err
	}

	return metav1.Time{
		Time: iTime,
	}, nil
}
