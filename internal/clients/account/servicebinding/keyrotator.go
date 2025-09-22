package servicebindingclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

const ForceRotationKey = "servicebinding.account.btp.crossplane.io/force-rotation"

const iso8601Date = "2006-01-02T15:04:05Z0700"

const (
	errDeleteExpiredKey = "cannot delete expired key"
	errDeleteRetiredKey = "cannot delete retired key"
)

// InstanceDeleter provides the interface for deleting service binding instances
type InstanceDeleter interface {
	DeleteInstance(ctx context.Context, cr *v1alpha1.ServiceBinding, targetName string, targetExternalName string) error
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
}

type SBKeyRotator struct {
	instanceDeleter InstanceDeleter
}

func NewSBKeyRotator(instanceDeleter InstanceDeleter) *SBKeyRotator {
	return &SBKeyRotator{
		instanceDeleter: instanceDeleter,
	}
}

func (r *SBKeyRotator) RetireBinding(cr *v1alpha1.ServiceBinding) bool {
	forceRotation := false
	if cr.ObjectMeta.Annotations != nil {
		_, forceRotation = cr.ObjectMeta.Annotations[ForceRotationKey]
	}

	var rotationDue bool
	if cr.Spec.ForProvider.Rotation != nil && cr.Status.AtProvider.CreatedDate != nil {
		rotationDue = cr.Status.AtProvider.CreatedDate.Add(cr.Spec.ForProvider.Rotation.Frequency.Duration).Before(time.Now())
	}

	if forceRotation || rotationDue {
		for _, retiredKey := range cr.Status.AtProvider.RetiredKeys {
			if retiredKey.ID == cr.Status.AtProvider.ID {
				// If the binding is already retired, do not retire it again.
				return true
			}
		}

		var createdDate v1.Time
		if cr.Status.AtProvider.CreatedDate != nil {
			createdDate = *cr.Status.AtProvider.CreatedDate
		}

		retiredKey := &v1alpha1.RetiredSBResource{
			ID:          cr.Status.AtProvider.ID,
			Name:        cr.Status.AtProvider.Name,
			CreatedDate: createdDate,
			RetiredDate: v1.Now(),
		}
		cr.Status.AtProvider.RetiredKeys = append(cr.Status.AtProvider.RetiredKeys, retiredKey)
		cr.Status.AtProvider.CreatedDate = nil

		return true
	}

	return false
}

func (r *SBKeyRotator) HasExpiredKeys(cr *v1alpha1.ServiceBinding) bool {
	if cr.Status.AtProvider.RetiredKeys == nil || cr.Spec.ForProvider.Rotation == nil ||
		cr.Spec.ForProvider.Rotation.TTL == nil {
		return false
	}

	for _, key := range cr.Status.AtProvider.RetiredKeys {
		if key.RetiredDate.IsZero() {
			continue
		}
		gracePeriod := cr.Spec.ForProvider.Rotation.TTL.Duration - cr.Spec.ForProvider.Rotation.Frequency.Duration
		if key.RetiredDate.Add(gracePeriod).Before(time.Now()) {
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

	for _, key := range cr.Status.AtProvider.RetiredKeys {
		// Keep the key if it's not expired yet or if instance tracking info is missing
		var expired bool
		if !key.RetiredDate.IsZero() && cr.Spec.ForProvider.Rotation.TTL != nil {
			gracePeriod := cr.Spec.ForProvider.Rotation.TTL.Duration - cr.Spec.ForProvider.Rotation.Frequency.Duration
			expired = key.RetiredDate.Add(gracePeriod).Before(time.Now())
		}

		if !expired || key.Name == "" {
			newRetiredKeys = append(newRetiredKeys, key)
			continue
		}

		if err := r.instanceDeleter.DeleteInstance(ctx, cr, key.Name, key.ID); err != nil {
			// If we cannot delete the key, keep it in the list
			newRetiredKeys = append(newRetiredKeys, key)
			errs = append(errs, fmt.Errorf("%s %s: %w", errDeleteExpiredKey, key.ID, err))
		}
	}

	return newRetiredKeys, errors.Join(errs...)
}

func (r *SBKeyRotator) DeleteRetiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) error {
	for _, retiredKey := range cr.Status.AtProvider.RetiredKeys {
		if err := r.instanceDeleter.DeleteInstance(ctx, cr, retiredKey.Name, retiredKey.ID); err != nil {
			return fmt.Errorf("%s %s: %w", errDeleteRetiredKey, retiredKey.ID, err)
		}
	}
	return nil
}
