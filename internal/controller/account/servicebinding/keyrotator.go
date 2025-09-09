package servicebinding

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

const ForceRotationKey = "servicebinding.account.btp.crossplane.io/force-rotation"

const iso8601Date = "2006-01-02T15:04:05Z0700"

type KeyRotator interface {
	// RetireBinding checks if the binding should be retired based on the rotation frequency
	// and the force rotation annotation. If it should be retired, it adds the retired key to the status.
	RetireBinding(cr *v1alpha1.ServiceBinding) bool

	// HasExpiredKeys checks if there are any retired keys that have expired based on the rotation TTL.
	HasExpiredKeys(cr *v1alpha1.ServiceBinding) bool

	// DeleteExpiredKeys deletes the expired keys from the status and the external system.
	// It returns the new list of retired keys and any error encountered during deletion.
	DeleteExpiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) ([]*v1alpha1.RetiredSBResource, error)

	// DeleteRetiredKeys deletes all retired keys from the external system.
	DeleteRetiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) error
}

type SBKeyRotator struct {
	external *external
}

func (r *SBKeyRotator) RetireBinding(cr *v1alpha1.ServiceBinding) bool {
	forceRotation := false
	if cr.ObjectMeta.Annotations != nil {
		_, forceRotation = cr.ObjectMeta.Annotations[ForceRotationKey]
	}

	var rotationDue bool
	if cr.Spec.ForProvider.Rotation != nil && cr.Spec.ForProvider.Rotation.Frequency != nil {
		if cr.Status.AtProvider.CreatedDate != nil {
			if createdTime, err := time.Parse(iso8601Date, *cr.Status.AtProvider.CreatedDate); err == nil {
				rotationDue = createdTime.Add(cr.Spec.ForProvider.Rotation.Frequency.Duration).Before(time.Now())
			}
		}
	}

	if forceRotation || rotationDue {
		// If the binding was created before the rotation frequency, retire it.
		for _, retiredKey := range cr.Status.AtProvider.RetiredKeys {
			if retiredKey.InstanceUID == cr.Status.AtProvider.InstanceUID {
				// If the binding is already retired, do not retire it again.
				return true
			}
		}

		// Store current binding instance info for tracking
		currentInstanceName := cr.Status.AtProvider.InstanceName
		currentInstanceUID := cr.Status.AtProvider.InstanceUID

		// If not set, use default values for backward compatibility
		if currentInstanceName == "" {
			currentInstanceName = cr.Name
		}
		if currentInstanceUID == "" {
			currentInstanceUID = string(cr.UID)
		}

		retiredKey := &v1alpha1.RetiredSBResource{
			ID:           cr.Status.AtProvider.ID,
			Name:         cr.Status.AtProvider.Name,
			CreatedDate:  cr.Status.AtProvider.CreatedDate,
			InstanceName: currentInstanceName,
			InstanceUID:  currentInstanceUID,
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
		if key.CreatedDate != nil {
			if createdTime, err := time.Parse(iso8601Date, *key.CreatedDate); err == nil &&
				createdTime.Add(cr.Spec.ForProvider.Rotation.TTL.Duration).Before(time.Now()) {
				return true
			}
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
		if key.CreatedDate != nil && cr.Spec.ForProvider.Rotation.TTL != nil {
			if createdTime, err := time.Parse(iso8601Date, *key.CreatedDate); err == nil {
				expired = createdTime.Add(cr.Spec.ForProvider.Rotation.TTL.Duration).Before(time.Now())
			}
		}

		if !expired || key.InstanceName == "" || key.InstanceUID == "" {
			newRetiredKeys = append(newRetiredKeys, key)
			continue
		}

		// Try to delete the expired key using our deleteInstance function
		if err := r.external.deleteInstance(ctx, cr, key.InstanceName, types.UID(key.InstanceUID), key.ID); err != nil {
			// If we cannot delete the key, keep it in the list
			newRetiredKeys = append(newRetiredKeys, key)
			errs = append(errs, fmt.Errorf("cannot delete expired key %s: %w", key.ID, err))
		}
		// If deletion successful, don't add to newRetiredKeys (it gets removed)
	}

	return newRetiredKeys, errors.Join(errs...)
}

func (r *SBKeyRotator) DeleteRetiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) error {
	for _, retiredKey := range cr.Status.AtProvider.RetiredKeys {
		if retiredKey.InstanceName == "" || retiredKey.InstanceUID == "" {
			continue // Skip keys without proper instance tracking
		}

		if err := r.external.deleteInstance(ctx, cr, retiredKey.InstanceName, types.UID(retiredKey.InstanceUID), retiredKey.ID); err != nil {
			return fmt.Errorf("cannot delete retired key %s: %w", retiredKey.ID, err)
		}
	}
	return nil
}
