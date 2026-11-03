package v1alpha1

import (
	"time"

	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
)

// NextPollDeadline returns when Observe must next run to rotate the active
// binding or drop an expired one. Keep in sync with validateBindings in
// internal/controller/kymaenvironmentbinding.
func (cr *KymaEnvironmentBinding) NextPollDeadline() (time.Time, bool) {
	var ts []time.Time
	for _, b := range cr.Status.AtProvider.Bindings {
		ts = append(ts, b.ExpiresAt.Time)
		if b.IsActive {
			ts = append(ts, b.CreatedAt.Add(cr.Spec.ForProvider.RotationInterval.Duration))
		}
	}
	return providerv1alpha1.Earliest(ts...)
}
