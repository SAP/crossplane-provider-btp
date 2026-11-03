package v1alpha1

import (
	"time"

	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
)

// NextPollDeadline returns when Observe must next run to rotate the active key
// or delete a retired one. Keep in sync with NeedRetirement and HasExpiredKeys
// in internal/clients/account/servicebinding/keyrotator.go.
func (cr *ServiceBinding) NextPollDeadline() (time.Time, bool) {
	var ts []time.Time
	rot := cr.Spec.Rotation
	if rot != nil && rot.Frequency != nil && cr.Status.AtProvider.CreatedDate != nil {
		ts = append(ts, cr.Status.AtProvider.CreatedDate.Add(rot.Frequency.Duration))
	}
	for _, key := range cr.Status.RetiredKeys {
		if key == nil || key.RetiredDate.IsZero() {
			continue
		}
		switch {
		case rot != nil && rot.TTL != nil && rot.Frequency != nil:
			// The controller recomputes DeletionDate from the current
			// rotation settings before comparing.
			ts = append(ts, key.RetiredDate.Add(rot.TTL.Duration-rot.Frequency.Duration))
		case key.DeletionDate != nil:
			ts = append(ts, key.DeletionDate.Time)
		}
	}
	return providerv1alpha1.Earliest(ts...)
}
