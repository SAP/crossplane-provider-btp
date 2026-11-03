package v1alpha1

import (
	"time"

	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NextPollDeadline returns when Observe must next run to renew the JWT: its
// rotation start or its expiry, whichever comes first.
func (cr *CertBasedOIDCLogin) NextPollDeadline() (time.Time, bool) {
	var ts []time.Time
	for _, t := range []*metav1.Time{cr.Status.AtProvider.RotationNotBefore, cr.Status.AtProvider.ExpiresAt} {
		if t != nil {
			ts = append(ts, t.Time)
		}
	}
	return providerv1alpha1.Earliest(ts...)
}
