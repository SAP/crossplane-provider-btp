package servicebindingclient
//
// import (
// 	"context"
// 	"errors"
// 	"fmt"
// 	"net/http"
// 	"time"
//
// 	"github.com/crossplane/crossplane-runtime/pkg/meta"
// 	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
// 	"sigs.k8s.io/controller-runtime/pkg/client"
//
// 	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
// 	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
// )
//
// const ForceRotationKey = "servicecredentialbinding.cloudfoundry.crossplane.io/force-rotation"
//
// type KeyRotator interface {
// 	// RetireBinding checks if the binding should be retired baset on the rotation frequency
// 	// and the force rotation annotation. If it should be retired, it adds the retired key to the status.
// 	RetireBinding(cr *v1alpha1.ServiceBinding) bool
//
// 	// HasExpiredKeys checks if there are any retired keys that have expired based on the rotation TTL.
// 	HasExpiredKeys(cr *v1alpha1.ServiceBinding) bool
//
// 	// DeleteExpiredKeys deletes the expired keys from the status and the external system.
// 	// It returns the new list of retired keys and any error encountered during deletion.
// 	DeleteExpiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) ([]*v1alpha1.SBResource, error)
//
// 	// DeleteRetiredKeys deletes all retired keys from the external system.
// 	DeleteRetiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) error
// }
//
// type SBKeyRotator struct {
// 	client managed.ExternalClient
// 	kube   client.Client
// 	mapper tfclient.TfMapper[*v1alpha1.ServiceBinding, *v1alpha1.SubaccountServiceBinding]
// }
//
// func NewSBKeyRotator(client managed.ExternalClient, kube client.Client, mapper tfclient.TfMapper[*v1alpha1.ServiceBinding, *v1alpha1.SubaccountServiceBinding]) SBKeyRotator {
// 	return SBKeyRotator{
// 		kube: kube,
// 		client: client,
// 		mapper: mapper,
// 	}
// }
//
// func (r *SBKeyRotator) RetireBinding(cr *v1alpha1.ServiceBinding) bool {
// 	forceRotation := false
// 	if cr.ObjectMeta.Annotations != nil {
// 		_, forceRotation = cr.ObjectMeta.Annotations[ForceRotationKey]
// 	}
//
// 	rotationDue := cr.Spec.ForProvider.Rotation != nil && cr.Spec.ForProvider.Rotation.Frequency != nil &&
// 		(cr.Status.AtProvider.CreatedAt == nil ||
// 			cr.Status.AtProvider.CreatedAt.Add(cr.Spec.ForProvider.Rotation.Frequency.Duration).Before(time.Now()))
//
// 	if forceRotation || rotationDue {
// 		// If the binding was created before the rotation frequency, retire it.
// 		for _, retiredKey := range cr.Status.AtProvider.RetiredKeys {
// 			if retiredKey.ID == cr.Status.AtProvider.ID {
// 				// If the binding is already retired, do not retire it again.
// 				return true
// 			}
// 		}
// 		cr.Status.AtProvider.RetiredKeys = append(cr.Status.AtProvider.RetiredKeys, &v1alpha1.SBResource{
// 			ID:        cr.Status.AtProvider.ID,
// 			CreatedAt: cr.Status.AtProvider.CreatedAt,
// 		})
// 		return true
// 	}
//
// 	return false
// }
//
// func (r *SBKeyRotator) HasExpiredKeys(cr *v1alpha1.ServiceBinding) bool {
// 	if cr.Status.AtProvider.RetiredKeys == nil || cr.Spec.ForProvider.Rotation == nil ||
// 		cr.Spec.ForProvider.Rotation.TTL == nil {
// 		return false
// 	}
//
// 	for _, key := range cr.Status.AtProvider.RetiredKeys {
// 		if key.CreatedAt.Add(cr.Spec.ForProvider.Rotation.TTL.Duration).Before(time.Now()) {
// 			return true
// 		}
// 	}
//
// 	return false
// }
//
// func (c *SBKeyRotator) DeleteExpiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) ([]*v1alpha1.SBResource, error) {
// 	var newRetiredKeys []*v1alpha1.SBResource
// 	var errs []error
//
// 	for _, key := range cr.Status.AtProvider.RetiredKeys {
//
// 		if key.CreatedAt.Add(cr.Spec.ForProvider.Rotation.TTL.Duration).After(time.Now()) ||
// 			key.ID == meta.GetExternalName(cr) {
// 			newRetiredKeys = append(newRetiredKeys, key)
//
// 		} else if err := c.delete(ctx, cr, key); err != nil {
//
// 			// If we cannot delete the key, keep it in the list
// 			newRetiredKeys = append(newRetiredKeys, key)
// 			errs = append(errs, fmt.Errorf("cannot delete expired key %s: %w", key.ID, err))
// 		}
// 	}
//
// 	return newRetiredKeys, errors.Join(errs...)
// }
//
// func (c *SBKeyRotator) DeleteRetiredKeys(ctx context.Context, cr *v1alpha1.ServiceBinding) error {
// 	for _, retiredKey := range cr.Status.AtProvider.RetiredKeys {
// 		if err := c.delete(ctx, cr, retiredKey); err != nil {
// 			return fmt.Errorf("cannot delete retired key %s: %w", retiredKey.ID, err)
// 		}
// 	}
// 	return nil
// }
//
// func (c *SBKeyRotator) delete(ctx context.Context, cr *v1alpha1.ServiceBinding, el *v1alpha1.SBResource) error {
// 	crr := cr.DeepCopy()
// 	crr.SetAnnotations(map[string]string{
// 		"crossplane.io/external-name": el.ID,
// 	})
// 	crr.Spec.ForProvider.Name = el.Name
// 	r, err := c.mapper.TfResource(ctx, crr, c.kube)
// 	if err != nil {
// 		return fmt.Errorf("cannot map resource: %w", err)
// 	}
// 	if _, err := c.client.Delete(ctx, r); err != nil {
// 		return fmt.Errorf("cannot delete resource: %w", err)
// 	}
// 	return nil
//
// }
//
// // Delete deletes a ServiceBinding resource
// func IsResourceNotFoundError(res *http.Response) bool {
// 	if res != nil && res.StatusCode == 404 {
// 		return true
// 	}
//
// 	return false
//
// }
