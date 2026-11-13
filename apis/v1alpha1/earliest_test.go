package v1alpha1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEarliest(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		ts     []time.Time
		want   time.Time
		wantOK bool
	}{
		"Empty":     {},
		"Single":    {ts: []time.Time{t0}, want: t0, wantOK: true},
		"Unordered": {ts: []time.Time{t0.Add(time.Hour), t0, t0.Add(time.Minute)}, want: t0, wantOK: true},
		"Equal":     {ts: []time.Time{t0, t0}, want: t0, wantOK: true},
		"PastFirst": {ts: []time.Time{t0.Add(-time.Hour), t0}, want: t0.Add(-time.Hour), wantOK: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := Earliest(tc.ts...)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}
