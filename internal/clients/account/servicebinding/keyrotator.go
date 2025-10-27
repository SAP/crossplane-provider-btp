package servicebindingclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
)

const ForceRotationKey = "servicebinding.account.btp.crossplane.io/force-rotation"

const (
	errDeleteExpiredKey = "cannot delete expired key"
	errDeleteRetiredKey = "cannot delete retired key"
)

// Condition types for ServiceBinding
const (
	TypeRotationStatus xpv1.ConditionType = "RotationStatus"
)

// BindingDeleter provides the interface for deleting service bindings
type BindingDeleter interface {
	DeleteBinding(ctx context.Context, cr *v1alpha1.ServiceBinding, targetName string, targetExternalName string) error
}

type KeyRotator interface {
	// RetireBinding checks if the binding should be retired based on the rotation frequency
	// and the force rotation annotation. If it should be retired, it adds the retired key to the status.
	RetireBinding(cr *v1alpha1.ServiceBinding) bool

	// HasExpiredKeys checks if there are any retired keys that have expired based on the rotation TTL.
	HasExpiredKeys(cr *v1alpha1.ServiceBinding) bool

	// IsCurrentBindingRetired checks if the current binding is already marked as retired.
	IsCurrentBindingRetired(cr *v1alpha1.ServiceBinding) bool

	// DeleteExpiredKeys deletes the expired keys from the status and the external system.
	// It returns the new list of retired keys and any error encountered during deletion.
	DeleteExpiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) ([]*v1alpha1.RetiredSBResource, error)

	// DeleteRetiredKeys deletes all retired keys from the external system.
	DeleteRetiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) error

	// ValidateRotationSettings checks the current rotation configuration and sets
	// the rotation status condition accordingly.
	ValidateRotationSettings(cr *v1alpha1.ServiceBinding)
}

type SBKeyRotator struct {
	bindingDeleter BindingDeleter
}

func NewSBKeyRotator(bindingDeleter BindingDeleter) *SBKeyRotator {
	return &SBKeyRotator{
		bindingDeleter: bindingDeleter,
	}
}

// isRotationConfigured checks if the service binding has valid rotation configuration
// and retired keys to process
func (r *SBKeyRotator) isRotationConfigured(cr *v1alpha1.ServiceBinding) bool {
	return cr.Spec.Rotation != nil
}

func (r *SBKeyRotator) RetireBinding(cr *v1alpha1.ServiceBinding) bool {
	forceRotation := v1.HasAnnotation(cr.ObjectMeta, ForceRotationKey)

	// If force rotation is requested but rotation is not configured, ignore the request
	// This prevents creating retired keys without proper deletion dates
	if forceRotation && !r.isRotationConfigured(cr) {
		return false
	}

	var rotationDue bool
	if r.isRotationConfigured(cr) && !cr.Status.AtProvider.CreatedDate.IsZero() {
		rotationDue = cr.Status.AtProvider.CreatedDate.Add(cr.Spec.Rotation.Frequency.Duration).Before(time.Now())
	}

	if !forceRotation && !rotationDue {
		return false
	}

	// If the binding is already retired, do not retire it again.
	for _, retiredKey := range cr.Status.AtProvider.RetiredKeys {
		if retiredKey.ID == cr.Status.AtProvider.ID {
			return true
		}
	}

	var createdDate v1.Time
	if cr.Status.AtProvider.CreatedDate != nil {
		createdDate = *cr.Status.AtProvider.CreatedDate
	}

	retiredKey := &v1alpha1.RetiredSBResource{
		ID:           cr.Status.AtProvider.ID,
		Name:         cr.Status.AtProvider.Name,
		CreatedDate:  createdDate,
		RetiredDate:  v1.Now(),
		DeletionDate: deletionDate(time.Now(), cr.Spec.Rotation),
	}
	cr.Status.AtProvider.RetiredKeys = append(cr.Status.AtProvider.RetiredKeys, retiredKey)

	return true
}

func deletionDate(base time.Time, r *v1alpha1.RotationParameters) *v1.Time {
	return internal.Ptr(v1.NewTime(base.Add(r.TTL.Duration - r.Frequency.Duration)))
}

// ValidateRotationSettings checks the current rotation configuration and sets
// the rotation status condition accordingly. This provides visibility into
// whether rotation is enabled, disabled, or has potentially problematic settings.
func (r *SBKeyRotator) ValidateRotationSettings(cr *v1alpha1.ServiceBinding) {
	forceRotation := v1.HasAnnotation(cr.ObjectMeta, ForceRotationKey)

	if !r.isRotationConfigured(cr) {
		message := "Key rotation is not configured for this service binding"
		if forceRotation {
			message += ". Force rotation annotation ignored - rotation parameters required to determine deletion schedule"
		}
		cr.Status.SetConditions(xpv1.Condition{
			Type:               TypeRotationStatus,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: v1.Now(),
			Reason:             "Disabled",
			Message:            message,
		})
		return
	}

	rotation := cr.Spec.Rotation
	ttl := rotation.TTL.Duration
	frequency := rotation.Frequency.Duration

	// Calculate potential maximum instances: TTL / frequency
	// We allow some floating point imprecision, so we check if it would result in more than 2 instances
	maxInstances := float64(ttl) / float64(frequency)

	if maxInstances > 2.0 {
		cr.Status.SetConditions(xpv1.Condition{
			Type:               TypeRotationStatus,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: v1.Now(),
			Reason:             "EnabledWithWarning",
			Message: fmt.Sprintf(
				"Key rotation is enabled but current settings (TTL=%s, frequency=%s) may result in up to %d parallel service binding instances. "+
					"Consider reducing TTL or increasing frequency to avoid resource overhead.",
				ttl.String(),
				frequency.String(),
				int(maxInstances),
			),
		})
	} else {
		cr.Status.SetConditions(xpv1.Condition{
			Type:               TypeRotationStatus,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: v1.Now(),
			Reason:             "Enabled",
		})
	}
}

func (r *SBKeyRotator) HasExpiredKeys(cr *v1alpha1.ServiceBinding) bool {
	if cr.Status.AtProvider.RetiredKeys == nil {
		return false
	}

	for _, key := range cr.Status.AtProvider.RetiredKeys {
		if key.RetiredDate.IsZero() {
			continue
		}

		// update DeletionDate in case rotation settings were changed
		if r.isRotationConfigured(cr) {
			key.DeletionDate = deletionDate(key.RetiredDate.Time, cr.Spec.Rotation)
		}

		if key.DeletionDate.Before(internal.Ptr(v1.Now())) {
			return true
		}
	}

	return false
}

func (r *SBKeyRotator) IsCurrentBindingRetired(cr *v1alpha1.ServiceBinding) bool {
	for _, retiredKey := range cr.Status.AtProvider.RetiredKeys {
		if retiredKey.ID == cr.Status.AtProvider.ID {
			return true
		}
	}
	return false
}

func (r *SBKeyRotator) DeleteExpiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) ([]*v1alpha1.RetiredSBResource, error) {
	var newRetiredKeys []*v1alpha1.RetiredSBResource
	var errs []error

	if cr.Status.AtProvider.RetiredKeys == nil {
		return newRetiredKeys, nil
	}

	for _, key := range cr.Status.AtProvider.RetiredKeys {
		// update DeletionDate in case rotation settings were changed
		if r.isRotationConfigured(cr) {
			key.DeletionDate = deletionDate(key.RetiredDate.Time, cr.Spec.Rotation)
		}

		if !key.DeletionDate.Before(internal.Ptr(v1.Now())) {
			newRetiredKeys = append(newRetiredKeys, key)
			continue
		}

		if err := r.bindingDeleter.DeleteBinding(ctx, cr, key.Name, key.ID); err != nil {
			// If we cannot delete the key, keep it in the list
			newRetiredKeys = append(newRetiredKeys, key)
			errs = append(errs, fmt.Errorf("%s %s: %w", errDeleteExpiredKey, key.ID, err))
		}
	}

	return newRetiredKeys, errors.Join(errs...)
}

func (r *SBKeyRotator) DeleteRetiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) error {
	for _, retiredKey := range cr.Status.AtProvider.RetiredKeys {
		if err := r.bindingDeleter.DeleteBinding(ctx, cr, retiredKey.Name, retiredKey.ID); err != nil {
			return fmt.Errorf("%s %s: %w", errDeleteRetiredKey, retiredKey.ID, err)
		}
	}
	return nil
}
