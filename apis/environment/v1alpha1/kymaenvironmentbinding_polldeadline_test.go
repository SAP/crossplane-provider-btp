package v1alpha1

import (
	"testing"
	"time"

	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKymaEnvironmentBindingNextPollDeadline(t *testing.T) {
	t0 := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) metav1.Time { return metav1.Time{Time: t0.Add(d)} }

	cases := map[string]struct {
		bindings []Binding
		want     time.Time
		wantOK   bool
	}{
		"NoBindings": {},
		"ActiveRotationBeforeExpiry": {
			bindings: []Binding{{IsActive: true, CreatedAt: at(0), ExpiresAt: at(2 * time.Hour)}},
			want:     t0.Add(time.Hour), wantOK: true,
		},
		"InactiveOnlyExpiry": {
			bindings: []Binding{{CreatedAt: at(-2 * time.Hour), ExpiresAt: at(30 * time.Minute)}},
			want:     t0.Add(30 * time.Minute), wantOK: true,
		},
		"EarliestAcrossBindings": {
			bindings: []Binding{
				{IsActive: true, CreatedAt: at(0), ExpiresAt: at(2 * time.Hour)},
				{CreatedAt: at(-time.Hour), ExpiresAt: at(15 * time.Minute)},
			},
			want: t0.Add(15 * time.Minute), wantOK: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := &KymaEnvironmentBinding{}
			cr.Spec.ForProvider.RotationInterval = providerv1alpha1.Duration{Duration: time.Hour}
			cr.Status.AtProvider.Bindings = tc.bindings
			got, ok := cr.NextPollDeadline()
			if ok != tc.wantOK || !got.Equal(tc.want) {
				t.Errorf("NextPollDeadline() = %v, %v; want %v, %v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
