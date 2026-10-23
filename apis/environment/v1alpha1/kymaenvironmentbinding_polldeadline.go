package v1alpha1

import "time"

// NextPollDeadline returns when Observe must next run to rotate the active
// binding or drop an expired one. Keep in sync with validateBindings in
// internal/controller/kymaenvironmentbinding.
func (cr *KymaEnvironmentBinding) NextPollDeadline() (time.Time, bool) {
	var at time.Time
	ok := false
	add := func(t time.Time) {
		if !ok || t.Before(at) {
			at, ok = t, true
		}
	}
	for _, b := range cr.Status.AtProvider.Bindings {
		add(b.ExpiresAt.Time)
		if b.IsActive {
			add(b.CreatedAt.Add(cr.Spec.ForProvider.RotationInterval.Duration))
		}
	}
	return at, ok
}
