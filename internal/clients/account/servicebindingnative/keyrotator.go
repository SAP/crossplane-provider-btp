package servicebindingnative

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/meta"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	servicemanagerclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
)

const ForceRotationKey = "servicecredentialbinding.cloudfoundry.crossplane.io/force-rotation"

type KeyRotator interface {
	// RetireBinding checks if the binding should be retired based on the rotation frequency
	// and the force rotation annotation. If it should be retired, it adds the retired key to the status.
	RetireBinding() bool

	// HasExpiredBindings checks if there are any retired keys that have expired based on the rotation TTL.
	HasExpiredBindings() bool

	// DeleteExpiredBindings deletes the expired keys from the status and the external system.
	// It returns the new list of retired keys and any error encountered during deletion.
	DeleteExpiredBindings(ctx context.Context) error

	// DeleteRetiredBindings deletes all retired keys from the external system.
	DeleteRetiredBindings(ctx context.Context) error
}

type SBKeyRotator struct {
	client *servicemanagerclient.APIClient
	cr     *v1alpha1.ServiceBinding
}

func NewSBKeyRotator(client *servicemanagerclient.APIClient, cr *v1alpha1.ServiceBinding) *SBKeyRotator {
	return &SBKeyRotator{
		client: client,
		cr:     cr,
	}
}

func (r *SBKeyRotator) RetireBinding() bool {
	forceRotation := false
	if r.cr.ObjectMeta.Annotations != nil {
		_, forceRotation = r.cr.ObjectMeta.Annotations[ForceRotationKey]
	}

	rotationDue := r.cr.Spec.ForProvider.Rotation != nil && r.cr.Spec.ForProvider.Rotation.Frequency != nil &&
		(r.cr.Status.AtProvider.CreatedAt == nil ||
			r.cr.Status.AtProvider.CreatedAt.Add(r.cr.Spec.ForProvider.Rotation.Frequency.Duration).Before(time.Now()))

	if forceRotation || rotationDue {
		// If the binding was created before the rotation frequency, retire it.
		for _, retiredKey := range r.cr.Status.AtProvider.RetiredKeys {
			if retiredKey.ID == r.cr.Status.AtProvider.ID {
				// If the binding is already retired, do not retire it again.
				return true
			}
		}
		r.cr.Status.AtProvider.RetiredKeys = append(r.cr.Status.AtProvider.RetiredKeys, &v1alpha1.SBResource{
			ID:        r.cr.Status.AtProvider.ID,
			CreatedAt: r.cr.Status.AtProvider.CreatedAt,
			Name:      r.cr.Status.AtProvider.Name,
		})
		return true
	}

	return false
}

func (r *SBKeyRotator) HasExpiredBindings() bool {
	if r.cr.Status.AtProvider.RetiredKeys == nil || r.cr.Spec.ForProvider.Rotation == nil ||
		r.cr.Spec.ForProvider.Rotation.TTL == nil {
		return false
	}

	for _, key := range r.cr.Status.AtProvider.RetiredKeys {
		if key.CreatedAt.Add(r.cr.Spec.ForProvider.Rotation.TTL.Duration).Before(time.Now()) {
			return true
		}
	}

	return false
}

func (c *SBKeyRotator) DeleteExpiredBindings(ctx context.Context) error {
	var newRetiredKeys []*v1alpha1.SBResource
	var errs []error

	for _, key := range c.cr.Status.AtProvider.RetiredKeys {

		if key.CreatedAt.Add(c.cr.Spec.ForProvider.Rotation.TTL.Duration).After(time.Now()) ||
			key.ID == meta.GetExternalName(c.cr) {
			newRetiredKeys = append(newRetiredKeys, key)

		} else if err := c.deleteBinding(ctx, key.ID); err != nil {
			// If we cannot delete the key, keep it in the list
			errs = append(errs, fmt.Errorf("cannot delete expired key %s: %w", key.ID, err))
			newRetiredKeys = append(newRetiredKeys, key)
		}
	}

	c.cr.Status.AtProvider.RetiredKeys = newRetiredKeys

	return errors.Join(errs...)
}

func (c *SBKeyRotator) DeleteRetiredBindings(ctx context.Context) error {
	for _, retiredKey := range c.cr.Status.AtProvider.RetiredKeys {
		if err := c.deleteBinding(ctx, retiredKey.ID); err != nil {
			return fmt.Errorf("cannot delete retired key %s: %w", retiredKey.ID, err)
		}
	}
	c.cr.Status.AtProvider.RetiredKeys = make([]*v1alpha1.SBResource, 0)
	return nil
}

func (c *SBKeyRotator) deleteBinding(ctx context.Context, id string) error {
	_, _, err := c.client.ServiceBindingsAPI.DeleteServiceBinding(ctx, id).Execute()
	if err != nil {
		if genericErr, ok := err.(*servicemanagerclient.GenericOpenAPIError); ok &&
			strings.Contains(genericErr.Error(), "404") {
			return nil
		}

		return specifyAPIError(err)
	}

	return nil
}
