package pollinterval

import (
	"math"
	"testing"
	"time"
)

func TestHook(t *testing.T) {
	cases := map[string]struct {
		pct  uint
		rnd  float64
		base time.Duration
		want time.Duration
	}{
		"ZeroPercentReturnsBase":        {pct: 0, rnd: 0, base: time.Minute, want: time.Minute},
		"MidpointReturnsBase":           {pct: 10, rnd: 0.5, base: time.Minute, want: time.Minute},
		"LowestDrawSubtractsFullJitter": {pct: 10, rnd: 0, base: time.Minute, want: 54 * time.Second},
		"UpperDrawAddsJitter":           {pct: 10, rnd: 0.75, base: time.Minute, want: 63 * time.Second},
		"HighestDrawStaysBelowBand":     {pct: 10, rnd: math.Nextafter(1, 0), base: time.Minute, want: 65999999999},
		"MaxPercentStaysPositive":       {pct: 99, rnd: 0, base: time.Minute, want: 600 * time.Millisecond},
		"ScalesWithShortAnnotationBase": {pct: 10, rnd: 0, base: 5 * time.Second, want: 4500 * time.Millisecond},
		"ScalesWithLargerPercentage":    {pct: 50, rnd: 0.25, base: 10 * time.Second, want: 7500 * time.Millisecond},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			hook := newHook(tc.pct, func() float64 { return tc.rnd })
			if got := hook(nil, tc.base); got != tc.want {
				t.Errorf("hook(%v) with pct=%d rnd=%v = %v, want %v", tc.base, tc.pct, tc.rnd, got, tc.want)
			}
		})
	}
}

func TestHookStaysWithinBand(t *testing.T) {
	const pct = 10
	hook := NewHook(pct)
	base := time.Minute
	lo, hi := base*(100-pct)/100, base*(100+pct)/100
	for range 1000 {
		if got := hook(nil, base); got < lo || got >= hi {
			t.Fatalf("hook(%v) = %v, want in [%v, %v)", base, got, lo, hi)
		}
	}
}
