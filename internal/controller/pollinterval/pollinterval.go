// Package pollinterval provides the provider-wide managed.PollIntervalHook.
package pollinterval

import (
	"math/rand/v2"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
)

// MinPollInterval is the floor for the crossplane.io/poll-interval
// annotation. It matches Crossplane core's --min-poll-interval default.
const MinPollInterval = time.Second

// NewHook returns the provider-wide PollIntervalHook. The runtime passes the
// effective poll interval (the annotation, floored at MinPollInterval, or
// --poll); the hook returns it offset by a random ±jitterPct% of itself.
func NewHook(jitterPct uint) managed.PollIntervalHook {
	return newHook(jitterPct, rand.Float64)
}

func newHook(jitterPct uint, rnd func() float64) managed.PollIntervalHook {
	if jitterPct == 0 {
		return func(_ resource.Managed, base time.Duration) time.Duration { return base }
	}
	pct := float64(jitterPct)
	return func(_ resource.Managed, base time.Duration) time.Duration {
		// Multiply before dividing so typical values stay exact in float64.
		return base + time.Duration(float64(base)*pct*(rnd()*2-1)/100)
	}
}
