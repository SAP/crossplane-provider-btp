package servicebinding

import (
	"context"
	"fmt"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	servicebindingclient "github.com/sap/crossplane-provider-btp/internal/clients/account/servicebinding"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	tfClient "github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/reconcilerutil"
	"github.com/sap/crossplane-provider-btp/internal/recovery"
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
	errSeedBinding          = "cannot initialize servicebinding state for deletion"
	errVerifyBinding        = "cannot verify servicebinding deletion"
	errCommitName           = "cannot persist pending servicebinding name"
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
	CreateClient(ctx context.Context, cr *v1alpha1.ServiceBinding, targetName string, targetExternalName string, markForDeletion bool) (servicebindingclient.ServiceBindingClientInterface, error)
}

// DefaultServiceBindingClientFactory is the production implementation
type DefaultServiceBindingClientFactory struct {
	kube        kubeclient.Client
	tfConnector servicebindingclient.TfConnector
}

func (f *DefaultServiceBindingClientFactory) CreateClient(ctx context.Context, cr *v1alpha1.ServiceBinding, targetName string, targetExternalName string, markForDeletion bool) (servicebindingclient.ServiceBindingClientInterface, error) {
	client, err := servicebindingclient.NewServiceBindingClient(ctx, f.kube, f.tfConnector, cr, targetName, targetExternalName, markForDeletion)
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

	// newAdminLookuperFn builds a SemanticLookuper backed by the subaccount-admin
	// SM binding (via the accounts-service), returning a cleanup func.
	newAdminLookuperFn func(ctx context.Context, cr *v1alpha1.ServiceBinding) (smClient.SemanticLookuper, func(), error)
	// recorder emits Kubernetes events for the heal path. May be nil.
	recorder event.Recorder
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

	client, err := c.clientFactory.CreateClient(ctx, cr, targetName, meta.GetExternalName(cr), false)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create client")
	}

	ext := &external{
		kube:          c.kube,
		clientFactory: c.clientFactory,
		tracker:       c.resourcetracker,
		client:        client,
		recorder:      c.recorder,
		nameGenerator: servicebindingclient.GenerateRandomName,
	}

	ext.keyRotator = c.newSBKeyRotatorFn(ext)
	ext.newAdminLookuperFn = c.newAdminLookuperFn

	return ext, nil
}

type external struct {
	kube          kubeclient.Client
	keyRotator    servicebindingclient.KeyRotator
	client        servicebindingclient.ServiceBindingClientInterface
	clientFactory ServiceBindingClientFactory
	tracker       tracking.ReferenceResolverTracker

	// newAdminLookuperFn builds the subaccount-admin-backed SemanticLookuper.
	newAdminLookuperFn func(ctx context.Context, cr *v1alpha1.ServiceBinding) (smClient.SemanticLookuper, func(), error)
	// recorder emits Kubernetes events for the heal path. May be nil.
	recorder event.Recorder
	// nameGenerator produces the BTP binding name for a fresh rotation
	// generation. Injected so tests can make the suffix deterministic; defaults
	// to servicebindingclient.GenerateRandomName in Connect.
	nameGenerator func(base string) string
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
		// Recovery: binding not found in BTP but with a fallback external-name.
		// Semantic lookup + ownership check; also covers the delete leg (heal
		// here so the next reconcile's Delete targets the real binding).
		if recovery.IsFallbackExternalName(cr.Name, meta.GetExternalName(cr)) {
			if healErr := e.healExternalName(ctx, cr); healErr != nil {
				return managed.ExternalObservation{}, healErr
			}
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

	// Resolve the BTP binding name for this Create attempt. resolveCreateName also performs the
	// lookup-before-create: if a binding under the committed name already exists
	// and is ours, it is adopted and adopted=true is returned so we skip the
	// create entirely.
	name, adopted, err := e.resolveCreateName(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBinding)
	}
	if adopted {
		// external-name is set and persisted; nothing was created this turn.
		return managed.ExternalCreation{}, nil
	}

	client, err := e.clientFactory.CreateClient(ctx, cr, name, name, false)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBinding)
	}

	e.client = client

	externalName, creation, err := client.Create(ctx)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBinding)
	}

	meta.SetExternalName(cr, externalName)
	// Clear the pending-name and force-rotation markers atomically with
	// persisting external-name: once external-name is durable the create result
	// is recorded, so the next reconcile must NOT regenerate a name.
	meta.RemoveAnnotations(cr, servicebindingclient.ForceRotationKey, servicebindingclient.PendingBindingNameKey)

	// Call the kube client to update the external-name and clear the annotations
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

// resolveCreateName decides the BTP binding name for the current Create attempt
// and makes that decision durable and idempotent across retries.
//
// It returns (name, adopted, err):
//   - adopted=true means an existing binding under the committed name was found
//     to be ours and its GUID has been set as external-name and persisted; the
//     caller must NOT create anything.
//   - adopted=false means the caller should create a binding named `name`.
//
// Flow:
//  1. Non-rotated bindings use the stable spec name directly — no random suffix,
//     nothing to persist, and no lookup needed (a name collision there is the
//     user's own doing, not a rotation artifact).
//  2. Rotated bindings reuse the name previously committed to the
//     PendingBindingNameKey annotation if present; otherwise a fresh name is
//     generated and persisted BEFORE any external call. Persisting first guarantees a
//     retried Create reuses the same name.
//  3. With a committed name in hand, a lookup-before-create adopts an existing
//     owned binding of that name (the previous attempt succeeded but lost its
//     result). Lookup errors are propagated so we retry rather than risk a
//     duplicate create.
func (e *external) resolveCreateName(ctx context.Context, cr *v1alpha1.ServiceBinding) (string, bool, error) {
	if !e.isRotationEnabled(cr) {
		return cr.Spec.ForProvider.Name, false, nil
	}

	name := cr.GetAnnotations()[servicebindingclient.PendingBindingNameKey]
	if name == "" {
		name = e.generateName(cr)
		meta.AddAnnotations(cr, map[string]string{servicebindingclient.PendingBindingNameKey: name})
		// Persist the committed name before creating anything in external system. Failing can return error since no leak happen.
		if err := e.kube.Update(ctx, cr); err != nil {
			return "", false, errors.Wrap(err, errCommitName)
		}
	}

	guid, found, err := e.lookupOwnedBinding(ctx, cr, name)
	if err != nil {
		return "", false, err
	}
	if !found {
		return name, false, nil
	}

	// Adopt the binding a prior attempt already created under this name.
	meta.SetExternalName(cr, guid)
	meta.RemoveAnnotations(cr, servicebindingclient.ForceRotationKey, servicebindingclient.PendingBindingNameKey)
	if err := e.kube.Update(ctx, cr); err != nil {
		return "", false, errors.Wrap(err, errCreateBinding)
	}
	log.FromContext(ctx).Info("adopted service binding created by a prior lost Create attempt", "guid", guid, "name", name)
	e.emit(cr, event.Normal(event.Reason(recovery.EventReasonRecovered),
		fmt.Sprintf("Adopted service binding %s created by a prior Create attempt (name=%s)", guid, name)))
	return name, true, nil
}

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

	var targetName string
	if cr.Status.AtProvider.Name != "" {
		targetName = cr.Status.AtProvider.Name
	} else {
		targetName = cr.Spec.ForProvider.Name
	}
	if err := e.DeleteBinding(ctx, cr, targetName, meta.GetExternalName(cr)); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteServiceBinding)
	}

	return managed.ExternalDelete{}, nil
}

// healExternalName performs the recovery for a ServiceBinding. The
// serviceInstanceID comes from the (already reference-resolved) parent
// instance's external-name; if the parent hasn't been recovered yet the
// lookup finds nothing and the next reconcile (after the parent recovers)
// succeeds.
func (e *external) healExternalName(ctx context.Context, cr *v1alpha1.ServiceBinding) error {
	if !recovery.HasCreateBeenAttempted(cr) {
		return nil
	}
	name := cr.Spec.ForProvider.Name
	if cr.Status.AtProvider.Name != "" {
		name = cr.Status.AtProvider.Name
	}

	guid, found, err := e.lookupOwnedBinding(ctx, cr, name)
	if err != nil {
		// Best-effort: lookupOwnedBinding already logged and emitted an event.
		// Do not block Observe; the next reconcile retries.
		return nil
	}
	if !found {
		return nil
	}

	meta.SetExternalName(cr, guid)
	if uErr := e.kube.Update(ctx, cr); uErr != nil {
		return errors.Wrap(uErr, "cannot persist recovered external-name")
	}

	log.FromContext(ctx).Info("recovered existing BTP service binding by external-name", "guid", guid, "serviceInstanceID", internal.Val(cr.Spec.ForProvider.ServiceInstanceID), "name", name)
	e.emit(cr, event.Normal(event.Reason(recovery.EventReasonRecovered),
		fmt.Sprintf("Recovered existing BTP service binding %s (semantic key: serviceInstanceID=%s name=%s)", guid, internal.Val(cr.Spec.ForProvider.ServiceInstanceID), name)))
	return recovery.ErrRequeueAfterRecovery
}

// lookupOwnedBinding runs the subaccount-admin semantic lookup for a binding
// named `name` under the CR's resolved parent service instance and returns its
// GUID only if it passes the ownership check — i.e. it was plausibly created by
// our own Create() attempt for this CR (recovery.IsOwnedByCR).
//
// found is true only for an owned match. A (false, nil) return means "no owned
// binding; the caller may safely create one". Lookup/config errors are logged,
// evented, and returned so each caller can decide: heal swallows them
// (best-effort), Create propagates them (retry rather than risk a duplicate).
func (e *external) lookupOwnedBinding(ctx context.Context, cr *v1alpha1.ServiceBinding, name string) (string, bool, error) {
	if e.newAdminLookuperFn == nil {
		return "", false, nil
	}
	serviceInstanceID := internal.Val(cr.Spec.ForProvider.ServiceInstanceID)
	if serviceInstanceID == "" {
		return "", false, nil
	}

	lookuper, cleanup, err := e.newAdminLookuperFn(ctx, cr)
	if err != nil {
		log.FromContext(ctx).Info("external-name recovery: cannot obtain admin lookup client", "error", err.Error())
		e.emit(cr, event.Warning(event.Reason(recovery.EventReasonLookupFailed), err))
		return "", false, err
	}
	defer cleanup()

	guid, createdAt, found, err := lookuper.LookupServiceBinding(ctx, serviceInstanceID, name)
	if err != nil {
		log.FromContext(ctx).Info("external-name recovery lookup failed", "serviceInstanceID", serviceInstanceID, "name", name, "error", err.Error())
		e.emit(cr, event.Warning(event.Reason(recovery.EventReasonLookupFailed), err))
		return "", false, err
	}
	if !found {
		return "", false, nil
	}

	if !recovery.IsOwnedByCR(cr, createdAt) {
		log.FromContext(ctx).Info("external-name recovery refused: BTP service binding is outside our Create-attempt window (brownfield)",
			"serviceInstanceID", serviceInstanceID, "name", name, "guid", guid,
			"crCreatedAt", cr.GetCreationTimestamp().Time, "btpCreatedAt", createdAt)
		e.emit(cr, event.Warning(
			event.Reason(recovery.EventReasonRefusedBrownfield),
			errors.Errorf(
				"refusing to recover existing BTP service binding %s: created_at %s is outside the window where our own Create() attempt for this CR could have produced it (brownfield). Set crossplane.io/external-name explicitly to import it (see external-name ADR)",
				guid, createdAt.Format(time.RFC3339))))
		return "", false, nil
	}

	return guid, true, nil
}

// emit records a Kubernetes event when a recorder is configured.
func (e *external) emit(cr resource.Managed, ev event.Event) {
	if e.recorder != nil {
		e.recorder.Event(cr, ev)
	}
}

// DeleteBinding implements the BindingDeleter interface for the key rotator.
//
// It performs a three-phase terraform-based deletion that is robust to the
// cold-workspace condition (setting delete on tf resource would not run a refresh, pjet detects the resource is missing and report successfully deleted, but the resource is still there in BTP) --> leaked service binding):
//
//	Phase 1 (seed):   Connect with markForDeletion=false. upjet's EnsureTFState
//	                  seeds terraform.tfstate with {id: <externalName>} because
//	                  WasDeleted is false. Without this, a container that never
//	                  observed this GUID while it was current has an empty state,
//	                  so destroy would run against nothing, destroy 0 and exit 0
//	                  (a silent no-op). This happens on a pod restart.
//	Phase 2 (destroy): Connect again with markForDeletion=true (same UID → same
//	                  workspace dir, so the seeded state persists). WasDeleted is
//	                  now true, so BuildMainTF sets prevent_destroy=false and the
//	                  destroy runs against the seeded state and actually deletes.
//	Phase 3 (verify): Connect once more with markForDeletion=false and Observe.
//	                  If the binding still exists, destroy silently no-op'd and we
//	                  return an error so the caller keeps the key.
func (e *external) DeleteBinding(ctx context.Context, cr *v1alpha1.ServiceBinding, targetName string, targetExternalName string) error {
	// Phase 1: seed the workspace state without marking for deletion.
	if _, err := e.clientFactory.CreateClient(ctx, cr, targetName, targetExternalName, false); err != nil {
		return errors.Wrap(err, errSeedBinding)
	}

	// Phase 2: connect with the deletion mark set so prevent_destroy is off, then destroy.
	client, err := e.clientFactory.CreateClient(ctx, cr, targetName, targetExternalName, true)
	if err != nil {
		return err
	}
	if _, err = client.Delete(ctx); err != nil {
		return err
	}

	// Phase 3: verify the external resource is actually gone before the caller
	// prunes any bookkeeping. A terraform destroy that no-op'd against an empty
	// state exits 0; only a positive read-back proves the binding was deleted.
	verifyClient, err := e.clientFactory.CreateClient(ctx, cr, targetName, targetExternalName, false)
	if err != nil {
		return errors.Wrap(err, errVerifyBinding)
	}
	observation, _, err := verifyClient.Observe(ctx)
	if err != nil {
		// The read-back itself failed transiently; this does not prove the
		// binding still exists, so retry
		return errors.Wrap(
			fmt.Errorf("%s: %w", err.Error(), servicebindingclient.ErrVerifyTransient),
			errVerifyBinding,
		)
	}
	if observation.ResourceExists {
		return errors.Errorf("%s: binding %s still exists after destroy", errVerifyBinding, targetExternalName)
	}

	return nil
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
		gen := e.nameGenerator
		if gen == nil {
			gen = servicebindingclient.GenerateRandomName
		}
		return gen(cr.Spec.ForProvider.Name)
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
