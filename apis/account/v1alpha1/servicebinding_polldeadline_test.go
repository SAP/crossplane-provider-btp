package v1alpha1

import (
	"testing"
	"time"

	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServiceBindingNextPollDeadline(t *testing.T) {
	t0 := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *metav1.Time { return &metav1.Time{Time: t0.Add(d)} }
	dur := func(d time.Duration) *providerv1alpha1.Duration { return &providerv1alpha1.Duration{Duration: d} }
	retired := func(retiredAgo, storedIn time.Duration) *RetiredSBResource {
		return &RetiredSBResource{RetiredDate: *at(-retiredAgo), DeletionDate: at(storedIn)}
	}

	cases := map[string]struct {
		rot     *RotationParameters
		created *metav1.Time
		keys    []*RetiredSBResource
		want    time.Time
		wantOK  bool
	}{
		"NoRotationNoKeys": {},
		"RotationDue": {
			rot: &RotationParameters{Frequency: dur(time.Hour)}, created: at(0),
			want: t0.Add(time.Hour), wantOK: true,
		},
		"RotationNilCreatedDate": {rot: &RotationParameters{Frequency: dur(time.Hour)}},
		"RotationNilTTLUsesStoredDeletionDate": {
			rot: &RotationParameters{Frequency: dur(time.Hour)}, created: at(0),
			keys: []*RetiredSBResource{retired(10*time.Minute, 20*time.Minute)},
			want: t0.Add(20 * time.Minute), wantOK: true,
		},
		"RecomputedFromRotation": {
			rot: &RotationParameters{Frequency: dur(time.Hour), TTL: dur(3 * time.Hour)}, created: at(0),
			keys: []*RetiredSBResource{retired(90*time.Minute, 5*time.Hour)},
			want: t0.Add(30 * time.Minute), wantOK: true,
		},
		"EarliestKeyWins": {
			keys: []*RetiredSBResource{retired(time.Hour, 2*time.Hour), retired(time.Hour, 30*time.Minute)},
			want: t0.Add(30 * time.Minute), wantOK: true,
		},
		"ZeroRetiredDateSkipped": {
			keys: []*RetiredSBResource{{DeletionDate: at(time.Hour)}},
		},
		"NilFrequency": {
			rot: &RotationParameters{TTL: dur(time.Hour)}, created: at(0),
			keys: []*RetiredSBResource{{RetiredDate: *at(0)}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := &ServiceBinding{}
			cr.Spec.Rotation = tc.rot
			cr.Status.AtProvider.CreatedDate = tc.created
			cr.Status.RetiredKeys = tc.keys
			got, ok := cr.NextPollDeadline()
			if ok != tc.wantOK || !got.Equal(tc.want) {
				t.Errorf("NextPollDeadline() = %v, %v; want %v, %v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
