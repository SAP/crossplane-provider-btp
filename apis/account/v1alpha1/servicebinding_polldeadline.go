package v1alpha1

import "time"

// NextPollDeadline returns when Observe must next run to rotate the active key
// or delete a retired one. Keep in sync with NeedRetirement and HasExpiredKeys
// in internal/clients/account/servicebinding/keyrotator.go.
func (cr *ServiceBinding) NextPollDeadline() (time.Time, bool) {
	var at time.Time
	ok := false
	add := func(t time.Time) {
		if !ok || t.Before(at) {
			at, ok = t, true
		}
	}
	rot := cr.Spec.Rotation
	if rot != nil && rot.Frequency != nil && cr.Status.AtProvider.CreatedDate != nil {
		add(cr.Status.AtProvider.CreatedDate.Add(rot.Frequency.Duration))
	}
	for _, key := range cr.Status.RetiredKeys {
		if key == nil || key.RetiredDate.IsZero() {
			continue
		}
		switch {
		case rot != nil && rot.TTL != nil && rot.Frequency != nil:
			// The controller recomputes DeletionDate from the current
			// rotation settings before comparing.
			add(key.RetiredDate.Add(rot.TTL.Duration - rot.Frequency.Duration))
		case key.DeletionDate != nil:
			add(key.DeletionDate.Time)
		}
	}
	return at, ok
}
