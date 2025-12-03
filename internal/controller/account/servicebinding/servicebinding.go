package servicebinding

import (
	"context"
	"encoding/json"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

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
	usage             resource.Tracker
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
		return observation, nil
	}

	if observation.ResourceUpToDate && tfResource != nil {
		if err := e.updateServiceBindingFromTfResource(cr, tfResource); err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errObserveSaveBinding)
		}

		if err := e.kube.Status().Update(ctx, cr); err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errUpdateStatus)
		}
	}

	observation.ConnectionDetails, err = flattenSecretData(observation.ConnectionDetails)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errFlattenSecret)
	}

	observation.ResourceUpToDate = observation.ResourceUpToDate && !e.keyRotator.HasExpiredKeys(cr)

	// Validate rotation settings and set status condition
	e.keyRotator.ValidateRotationSettings(cr)

	// Retire binding conditionally
	if !e.keyRotator.RetireBinding(cr) {
		if !cr.GetDeletionTimestamp().IsZero() {
			return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
		}
		return observation, nil
	}

	if err := e.kube.Status().Update(ctx, cr); err != nil {
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

	cr.Status.RetiredKeys = newRetiredKeys

	// store the result in the status even if errors are returned,
	// to remove keys for those where deletion was successfull
	if err := e.kube.Status().Update(ctx, cr); err != nil {
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

// flattenSecretData takes a map[string][]byte and flattens any JSON object values into the result map.
// For each key whose value is a JSON object, its keys/values are added to the result map as top-level entries.
// Non-JSON values are kept as-is.
func flattenSecretData(secretData map[string][]byte) (map[string][]byte, error) {
	result := make(map[string][]byte)
	for k, v := range secretData {
		var jsonMap map[string]any
		if err := json.Unmarshal(v, &jsonMap); err == nil {
			for jk, jv := range jsonMap {
				switch val := jv.(type) {
				case string:
					result[jk] = []byte(val)
				default:
					b, err := json.Marshal(val)
					if err != nil {
						return nil, err
					}
					result[jk] = b
				}
			}
		} else {
			result[k] = v
		}
	}
	return result, nil
}
