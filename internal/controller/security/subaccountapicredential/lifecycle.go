package subaccountapicredential

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/upjet/v2/pkg/config"
	"github.com/crossplane/upjet/v2/pkg/controller"
	"github.com/crossplane/upjet/v2/pkg/metrics"
	"github.com/crossplane/upjet/v2/pkg/terraform"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errAdoptionUnsupported  = "adopting an existing SubaccountApiCredential is not supported; choose a new credential name or intentionally remove the existing BTP credential"
	errOwnershipUncertain   = "SubaccountApiCredential ownership is uncertain; external operations are blocked until an administrator resolves ownership"
	errCredentialRetargeted = "SubaccountApiCredential identity differs from its established acquisition identity; in-place retargeting is not supported"
	defaultCredentialName   = "managed-subaccount-api-credential"
)

// NewGuardedConnector creates the normal Upjet connector and wraps its per-MR
// external client with UID-bound acquisition checks.
func NewGuardedConnector(kube client.Client, setup terraform.SetupFn, cfg *config.Resource, operations *controller.OperationTrackerStore, logger logging.Logger, recorder *metrics.MetricRecorder, managementPolicies bool) managed.ExternalConnector {
	base := controller.NewTerraformPluginFrameworkConnector(kube, setup, cfg, operations,
		controller.WithTerraformPluginFrameworkLogger(logger),
		controller.WithTerraformPluginFrameworkMetricRecorder(recorder),
		controller.WithTerraformPluginFrameworkManagementPolicies(managementPolicies))
	return &lifecycleConnector{kube: kube, base: base}
}

type lifecycleConnector struct {
	kube client.Client
	base managed.ExternalConnector
}

func (c *lifecycleConnector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*securityv1alpha1.SubaccountApiCredential)
	if !ok {
		return nil, errors.New("managed resource is not SubaccountApiCredential")
	}
	phase, err := c.ensureLifecycle(ctx, cr)
	if err != nil {
		return nil, err
	}
	switch phase {
	case securityv1alpha1.SubaccountApiCredentialPhaseRejected:
		if meta.WasDeleted(cr) {
			return &rejectedExternalClient{}, nil
		}
		return nil, errors.New(errAdoptionUnsupported)
	case securityv1alpha1.SubaccountApiCredentialPhaseUncertain, securityv1alpha1.SubaccountApiCredentialPhaseCreatePending:
		return nil, errors.New(errOwnershipUncertain)
	case securityv1alpha1.SubaccountApiCredentialPhaseOwned:
		if err := validateOwnedIdentity(cr); err != nil {
			return nil, err
		}
	case securityv1alpha1.SubaccountApiCredentialPhaseEligible:
		// Eligible resources may only perform the guarded read/Create flow below.
	default:
		return nil, errors.New(errOwnershipUncertain)
	}
	ec, err := c.base.Connect(ctx, mg)
	if err != nil {
		return nil, err
	}
	return &lifecycleExternalClient{kube: c.kube, base: ec}, nil
}

type lifecycleExternalClient struct {
	kube client.Client
	base managed.ExternalClient
}

func (c *lifecycleExternalClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr := mg.(*securityv1alpha1.SubaccountApiCredential)
	phase := cr.Status.Lifecycle.Phase
	if phase == securityv1alpha1.SubaccountApiCredentialPhaseRejected {
		return managed.ExternalObservation{}, errors.New(errAdoptionUnsupported)
	}
	if phase != securityv1alpha1.SubaccountApiCredentialPhaseEligible && phase != securityv1alpha1.SubaccountApiCredentialPhaseOwned {
		return managed.ExternalObservation{}, errors.New(errOwnershipUncertain)
	}
	before := cr.DeepCopy()
	observed, err := c.base.Observe(ctx, mg)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	if phase == securityv1alpha1.SubaccountApiCredentialPhaseEligible && observed.ResourceExists {
		// Upjet may have copied existing public fields into status, late-initialized
		// spec, or recovered the external name. Restore all managed data before
		// persisting only the terminal rejection receipt.
		restorePreObserveState(cr, before)
		if err := c.markRejected(ctx, cr, "existing-target-without-acquisition-receipt"); err != nil {
			return managed.ExternalObservation{}, err
		}
		return managed.ExternalObservation{}, errors.New(errAdoptionUnsupported)
	}
	if phase == securityv1alpha1.SubaccountApiCredentialPhaseOwned {
		if err := validateOwnedIdentity(cr); err != nil {
			restorePreObserveState(cr, before)
			return managed.ExternalObservation{}, err
		}
		if !observed.ResourceExists {
			// The receipt authorizes only the incarnation created by this MR. Once
			// BTP confirms absence, it cannot authorize deleting a later same-name
			// replacement. Require audited ownership resolution before any retry.
			restorePreObserveState(cr, before)
			cr.Status.Lifecycle.Phase = securityv1alpha1.SubaccountApiCredentialPhaseUncertain
			cr.Status.Lifecycle.Reason = "owned-target-observed-absent"
			if err := persistLifecycle(ctx, c.kube, cr); err != nil {
				return managed.ExternalObservation{}, err
			}
			return observed, nil
		}
	}
	return observed, nil
}

func restorePreObserveState(cr, before *securityv1alpha1.SubaccountApiCredential) {
	cr.Spec = before.Spec
	cr.Status = before.Status
	cr.SetAnnotations(before.GetAnnotations())
}

func (c *lifecycleExternalClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr := mg.(*securityv1alpha1.SubaccountApiCredential)
	if cr.Status.Lifecycle == nil || cr.Status.Lifecycle.Phase != securityv1alpha1.SubaccountApiCredentialPhaseEligible {
		return managed.ExternalCreation{}, errors.New(errOwnershipUncertain)
	}
	identity, err := identityFor(cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	if identity.subaccount != cr.Status.Lifecycle.SubaccountID || identity.name != cr.Status.Lifecycle.Name {
		return managed.ExternalCreation{}, errors.New(errCredentialRetargeted)
	}
	attempt := uuid.NewString()
	now := metav1.NewTime(time.Now().UTC())
	cr.Status.Lifecycle.Phase = securityv1alpha1.SubaccountApiCredentialPhaseCreatePending
	cr.Status.Lifecycle.AttemptID = attempt
	cr.Status.Lifecycle.AttemptedAt = &now
	cr.Status.Lifecycle.SubaccountID = identity.subaccount
	cr.Status.Lifecycle.Name = identity.name
	if err := persistLifecycle(ctx, c.kube, cr); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot persist credential Create intent; BTP Create was not issued")
	}
	created, createErr := c.base.Create(ctx, mg)
	if createErr != nil {
		// A collision is conclusive only for this known, single Create attempt.
		// It rejects acquisition, but does not grant any authority over the target.
		if strings.Contains(strings.ToLower(createErr.Error()), "already exists") {
			cr.Status.Lifecycle.Phase = securityv1alpha1.SubaccountApiCredentialPhaseRejected
			cr.Status.Lifecycle.Reason = "create-collision"
			if persistErr := persistLifecycle(ctx, c.kube, cr); persistErr != nil {
				return managed.ExternalCreation{}, errors.Wrapf(persistErr, "Create collided with an existing credential (%v), and rejection state could not be persisted", createErr)
			}
		}
		return managed.ExternalCreation{}, createErr
	}
	cr.Status.Lifecycle.Phase = securityv1alpha1.SubaccountApiCredentialPhaseOwned
	cr.Status.Lifecycle.Reason = "provider-create-succeeded"
	if err := persistOwnedReceiptAfterCreate(ctx, c.kube, cr); err != nil {
		// Never retry BTP Create because receipt persistence failed. Pending state
		// survives to force conservative resolution after a restart.
		return managed.ExternalCreation{}, errors.Wrap(err, "BTP Create succeeded but ownership receipt persistence failed; do not retry Create")
	}
	return created, nil
}

func (c *lifecycleExternalClient) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr := mg.(*securityv1alpha1.SubaccountApiCredential)
	if cr.Status.Lifecycle == nil || cr.Status.Lifecycle.Phase != securityv1alpha1.SubaccountApiCredentialPhaseOwned {
		return managed.ExternalUpdate{}, errors.New(errOwnershipUncertain)
	}
	if err := validateOwnedIdentity(cr); err != nil {
		return managed.ExternalUpdate{}, err
	}
	return c.base.Update(ctx, mg)
}

func (c *lifecycleExternalClient) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr := mg.(*securityv1alpha1.SubaccountApiCredential)
	if cr.Status.Lifecycle != nil && cr.Status.Lifecycle.Phase == securityv1alpha1.SubaccountApiCredentialPhaseRejected {
		return managed.ExternalDelete{}, nil
	}
	if cr.Status.Lifecycle == nil || cr.Status.Lifecycle.Phase != securityv1alpha1.SubaccountApiCredentialPhaseOwned {
		return managed.ExternalDelete{}, errors.New(errOwnershipUncertain)
	}
	if err := validateOwnedIdentity(cr); err != nil {
		return managed.ExternalDelete{}, err
	}
	return c.base.Delete(ctx, mg)
}

func (c *lifecycleExternalClient) Disconnect(ctx context.Context) error {
	return c.base.Disconnect(ctx)
}

type rejectedExternalClient struct{}

func (*rejectedExternalClient) Observe(_ context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	if meta.WasDeleted(mg) {
		// This MR never acquired an external resource. Report a local absence so
		// Crossplane can run its normal finalizer without touching BTP.
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	return managed.ExternalObservation{}, errors.New(errAdoptionUnsupported)
}
func (*rejectedExternalClient) Create(context.Context, resource.Managed) (managed.ExternalCreation, error) {
	return managed.ExternalCreation{}, errors.New(errAdoptionUnsupported)
}
func (*rejectedExternalClient) Update(context.Context, resource.Managed) (managed.ExternalUpdate, error) {
	return managed.ExternalUpdate{}, errors.New(errAdoptionUnsupported)
}
func (*rejectedExternalClient) Delete(context.Context, resource.Managed) (managed.ExternalDelete, error) {
	return managed.ExternalDelete{}, nil
}
func (*rejectedExternalClient) Disconnect(context.Context) error { return nil }

type credentialIdentity struct{ subaccount, name string }

func identityFor(cr *securityv1alpha1.SubaccountApiCredential) (credentialIdentity, error) {
	params, err := cr.GetMergedParameters(true)
	if err != nil {
		return credentialIdentity{}, errors.Wrap(err, "cannot resolve effective credential parameters")
	}
	id := credentialIdentity{}
	id.subaccount, _ = params["subaccount_id"].(string)
	name, _ := params["name"].(string)
	if externalName := meta.GetExternalName(cr); externalName != "" {
		if subaccount, credentialName, ok := splitCredentialExternalName(externalName); ok {
			if id.subaccount != "" && id.subaccount != subaccount {
				return id, errors.New(errCredentialRetargeted)
			}
			if name != "" && name != credentialName {
				return id, errors.New(errCredentialRetargeted)
			}
			id.subaccount, name = subaccount, credentialName
		} else if name == "" {
			name = externalName
		} else if name != externalName {
			return id, errors.New(errCredentialRetargeted)
		}
	}
	if name == "" {
		name = defaultCredentialName
	}
	if id.subaccount == "" {
		return id, fmt.Errorf("cannot establish SubaccountApiCredential identity from subaccount_id and name")
	}
	id.name = name
	return id, nil
}

func splitCredentialExternalName(value string) (string, string, bool) {
	if strings.Count(value, "/") != 1 {
		return "", "", false
	}
	subaccount, name, _ := strings.Cut(value, "/")
	return subaccount, name, subaccount != "" && name != ""
}

func validateOwnedIdentity(cr *securityv1alpha1.SubaccountApiCredential) error {
	lifecycle := cr.Status.Lifecycle
	if lifecycle == nil || string(cr.UID) != lifecycle.ManagedResourceUID {
		return errors.New(errOwnershipUncertain)
	}
	if lifecycle.Reason == "existing-target-without-acquisition-receipt" || lifecycle.Reason == "create-collision" {
		return errors.New(errAdoptionUnsupported)
	}
	switch lifecycle.Reason {
	case "provider-create-succeeded":
		if lifecycle.AttemptID == "" || lifecycle.AttemptedAt == nil {
			return errors.New(errOwnershipUncertain)
		}
	case "administrator-resolved-legacy":
		if lifecycle.ResolvedBy == "" || lifecycle.ResolvedAt == nil || lifecycle.ResolutionEvidence == "" {
			return errors.New("legacy ownership resolution is incomplete: resolver, timestamp, and independent evidence are required")
		}
	default:
		return errors.New(errOwnershipUncertain)
	}
	identity, err := identityFor(cr)
	if err != nil {
		return err
	}
	if identity.subaccount != lifecycle.SubaccountID || identity.name != lifecycle.Name {
		return errors.New(errCredentialRetargeted)
	}
	return nil
}

func (c *lifecycleConnector) ensureLifecycle(ctx context.Context, cr *securityv1alpha1.SubaccountApiCredential) (securityv1alpha1.SubaccountApiCredentialLifecyclePhase, error) {
	lifecycle := cr.Status.Lifecycle
	if lifecycle == nil {
		if hasPriorManagedState(cr) {
			cr.Status.Lifecycle = &securityv1alpha1.SubaccountApiCredentialLifecycleStatus{ManagedResourceUID: string(cr.UID), Phase: securityv1alpha1.SubaccountApiCredentialPhaseUncertain, Reason: "legacy-resource-requires-audited-ownership-resolution"}
			if err := persistLifecycle(ctx, c.kube, cr); err != nil {
				return "", err
			}
			return securityv1alpha1.SubaccountApiCredentialPhaseUncertain, nil
		}
		identity, err := identityFor(cr)
		if err != nil {
			return "", err
		}
		cr.Status.Lifecycle = &securityv1alpha1.SubaccountApiCredentialLifecycleStatus{
			ManagedResourceUID: string(cr.UID), SubaccountID: identity.subaccount, Name: identity.name,
			Phase: securityv1alpha1.SubaccountApiCredentialPhaseEligible,
		}
		if err := persistLifecycle(ctx, c.kube, cr); err != nil {
			return "", errors.Wrap(err, "cannot persist initial lifecycle classification")
		}
		lifecycle = cr.Status.Lifecycle
	}
	if lifecycle.ManagedResourceUID != string(cr.UID) {
		return "", errors.New("lifecycle receipt UID does not match this managed resource; ownership is uncertain")
	}
	switch lifecycle.Phase {
	case securityv1alpha1.SubaccountApiCredentialPhaseOwned:
		if err := validateOwnedIdentity(cr); err != nil {
			return "", err
		}
	case securityv1alpha1.SubaccountApiCredentialPhaseEligible:
		identity, err := identityFor(cr)
		if err != nil {
			return "", err
		}
		if lifecycle.SubaccountID == "" && lifecycle.Name == "" {
			lifecycle.SubaccountID, lifecycle.Name = identity.subaccount, identity.name
			if err := persistLifecycle(ctx, c.kube, cr); err != nil {
				return "", err
			}
		} else if identity.subaccount != lifecycle.SubaccountID || identity.name != lifecycle.Name {
			return "", errors.New(errCredentialRetargeted)
		}
	}
	return lifecycle.Phase, nil
}

func hasPriorManagedState(cr *securityv1alpha1.SubaccountApiCredential) bool {
	if len(cr.Status.Conditions) != 0 {
		return true
	}
	for _, key := range []string{meta.AnnotationKeyExternalCreatePending, meta.AnnotationKeyExternalCreateSucceeded, meta.AnnotationKeyExternalCreateFailed} {
		if _, exists := cr.GetAnnotations()[key]; exists {
			return true
		}
	}
	data, _ := json.Marshal(cr.Status.AtProvider)
	return strings.Trim(string(data), "{} \n\t") != ""
}

func (c *lifecycleExternalClient) markRejected(ctx context.Context, cr *securityv1alpha1.SubaccountApiCredential, reason string) error {
	cr.Status.Lifecycle.Phase = securityv1alpha1.SubaccountApiCredentialPhaseRejected
	cr.Status.Lifecycle.Reason = reason
	return persistLifecycle(ctx, c.kube, cr)
}

func persistLifecycle(ctx context.Context, kube client.Client, cr *securityv1alpha1.SubaccountApiCredential) error {
	current := &securityv1alpha1.SubaccountApiCredential{}
	if err := kube.Get(ctx, client.ObjectKeyFromObject(cr), current); err != nil {
		return errors.Wrap(err, "cannot reload managed resource for lifecycle status update")
	}
	if current.UID != cr.UID {
		return errors.New("managed resource UID changed while persisting lifecycle receipt")
	}
	if current.ResourceVersion != cr.ResourceVersion {
		if reflect.DeepEqual(current.Status.Lifecycle, cr.Status.Lifecycle) {
			cr.Status.Lifecycle = cloneLifecycleStatus(current.Status.Lifecycle)
			return nil
		}
		if !safeConcurrentLifecycleTransition(current.Status.Lifecycle, cr.Status.Lifecycle) {
			return errors.New("lifecycle status changed concurrently; refusing to overwrite the newer ownership state")
		}
	}
	current.Status.Lifecycle = cloneLifecycleStatus(cr.Status.Lifecycle)
	if err := kube.Status().Update(ctx, current); err != nil {
		if apierrors.IsConflict(err) {
			return errors.Wrap(err, "lifecycle status update conflicted; retry reconciliation")
		}
		return errors.Wrap(err, "cannot persist lifecycle status")
	}
	cr.ResourceVersion = current.ResourceVersion
	cr.Status.Lifecycle = cloneLifecycleStatus(current.Status.Lifecycle)
	return nil
}

func persistOwnedReceiptAfterCreate(ctx context.Context, kube client.Client, cr *securityv1alpha1.SubaccountApiCredential) error {
	for attempt := 0; attempt < 3; attempt++ {
		if err := persistLifecycle(ctx, kube, cr); err == nil {
			return nil
		}
		current := &securityv1alpha1.SubaccountApiCredential{}
		if err := kube.Get(ctx, client.ObjectKeyFromObject(cr), current); err != nil {
			return errors.Wrap(err, "cannot reload lifecycle state to retry successful Create receipt")
		}
		if current.UID != cr.UID || current.Status.Lifecycle == nil {
			return errors.New("successful Create receipt cannot be safely retried: MR UID or lifecycle state changed")
		}
		if current.Status.Lifecycle.Phase == securityv1alpha1.SubaccountApiCredentialPhaseOwned &&
			current.Status.Lifecycle.ManagedResourceUID == string(cr.UID) &&
			current.Status.Lifecycle.AttemptID == cr.Status.Lifecycle.AttemptID {
			cr.ResourceVersion = current.ResourceVersion
			cr.Status.Lifecycle = cloneLifecycleStatus(current.Status.Lifecycle)
			return nil
		}
		if current.Status.Lifecycle.Phase != securityv1alpha1.SubaccountApiCredentialPhaseCreatePending ||
			current.Status.Lifecycle.ManagedResourceUID != string(cr.UID) ||
			current.Status.Lifecycle.AttemptID != cr.Status.Lifecycle.AttemptID {
			return errors.New("successful Create receipt cannot be safely retried: persisted intent no longer matches this attempt")
		}
		cr.ResourceVersion = current.ResourceVersion
	}
	return errors.New("ownership receipt persistence retries exhausted after successful BTP Create")
}

func safeConcurrentLifecycleTransition(current, desired *securityv1alpha1.SubaccountApiCredentialLifecycleStatus) bool {
	if current == nil || desired == nil || current.ManagedResourceUID != desired.ManagedResourceUID {
		return false
	}
	if current.SubaccountID != desired.SubaccountID || current.Name != desired.Name {
		return false
	}
	switch current.Phase {
	case securityv1alpha1.SubaccountApiCredentialPhaseEligible:
		return desired.Phase == securityv1alpha1.SubaccountApiCredentialPhaseCreatePending || desired.Phase == securityv1alpha1.SubaccountApiCredentialPhaseRejected
	case securityv1alpha1.SubaccountApiCredentialPhaseCreatePending:
		return desired.AttemptID != "" && current.AttemptID == desired.AttemptID &&
			(desired.Phase == securityv1alpha1.SubaccountApiCredentialPhaseOwned || desired.Phase == securityv1alpha1.SubaccountApiCredentialPhaseRejected)
	default:
		return false
	}
}

func cloneLifecycleStatus(in *securityv1alpha1.SubaccountApiCredentialLifecycleStatus) *securityv1alpha1.SubaccountApiCredentialLifecycleStatus {
	if in == nil {
		return nil
	}
	out := *in
	if in.AttemptedAt != nil {
		out.AttemptedAt = in.AttemptedAt.DeepCopy()
	}
	if in.ResolvedAt != nil {
		out.ResolvedAt = in.ResolvedAt.DeepCopy()
	}
	return &out
}
