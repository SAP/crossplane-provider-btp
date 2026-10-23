package v1alpha1

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCertBasedOIDCLoginNextPollDeadline(t *testing.T) {
	t0 := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *metav1.Time { return &metav1.Time{Time: t0.Add(d)} }

	cases := map[string]struct {
		rotateAt, expiresAt *metav1.Time
		want                time.Time
		wantOK              bool
	}{
		"BothNil":                  {},
		"RotationNotBeforeEarlier": {rotateAt: at(time.Hour), expiresAt: at(3 * time.Hour), want: t0.Add(time.Hour), wantOK: true},
		"OnlyExpiresAt":            {expiresAt: at(3 * time.Hour), want: t0.Add(3 * time.Hour), wantOK: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := &CertBasedOIDCLogin{}
			cr.Status.AtProvider.RotationNotBefore = tc.rotateAt
			cr.Status.AtProvider.ExpiresAt = tc.expiresAt
			got, ok := cr.NextPollDeadline()
			if ok != tc.wantOK || !got.Equal(tc.want) {
				t.Errorf("NextPollDeadline() = %v, %v; want %v, %v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
