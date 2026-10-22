package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NextPollDeadline returns when Observe must next run to renew the JWT: its
// rotation start or its expiry, whichever comes first.
func (cr *CertBasedOIDCLogin) NextPollDeadline() (time.Time, bool) {
	var at time.Time
	ok := false
	for _, t := range []*metav1.Time{cr.Status.AtProvider.RotationNotBefore, cr.Status.AtProvider.ExpiresAt} {
		if t != nil && (!ok || t.Time.Before(at)) {
			at, ok = t.Time, true
		}
	}
	return at, ok
}
