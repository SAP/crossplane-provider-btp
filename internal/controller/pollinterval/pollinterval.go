// Package pollinterval provides the provider-wide managed.PollIntervalHook.
package pollinterval

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
)

// MinPollInterval is the floor for the crossplane.io/poll-interval
// annotation. It matches Crossplane core's --min-poll-interval default.
const MinPollInterval = time.Second

const (
	// A resource still Creating polls every creatingPollInterval, for at most
	// creatingWindow after Create so an outage does not pin it there.
	creatingWindow       = 10 * time.Minute
	creatingPollInterval = 15 * time.Second
)

// Config holds the provider-wide poll settings.
type Config struct {
	// Poll is --poll.
	Poll time.Duration
	// PollMax is the ceiling of the adaptive poll interval (--poll-max).
	PollMax time.Duration
	// PollMaxAfter is how long a resource must stay unchanged before its
	// interval reaches PollMax (--poll-max-after).
	PollMaxAfter time.Duration
	// JitterPercentage is the ±% random offset applied to every result.
	JitterPercentage uint
}

// AdaptivePollEnabled reports whether stable resources may poll less often
// than Poll. Fast polling of Creating resources does not depend on it.
func (c Config) AdaptivePollEnabled() bool {
	return c.PollMax > c.Poll
}

// Deadliner is implemented by managed resources whose Observe acts on a clock
// deadline (credential rotation, key expiry, token renewal).
type Deadliner interface {
	// NextPollDeadline returns the earliest time Observe must run to act on
	// time-based state. A past time pins the resource at Poll. false means
	// the resource has no deadline.
	NextPollDeadline() (time.Time, bool)
}

// NewHook returns the provider-wide PollIntervalHook. The runtime passes the
// effective poll interval (the annotation, floored at MinPollInterval, or
// --poll). The first matching rule picks the interval:
//
//  1. The crossplane.io/poll-interval annotation is set: base.
//  2. Synced is not True, or lags the generation: Poll.
//  3. Ready is Creating since less than creatingWindow: min(creatingPollInterval, Poll).
//  4. Adaptive polling is enabled and Ready and Synced are True: Poll doubling every
//     PollMaxAfter/log2(PollMax/Poll) since either last changed, capped at
//     PollMax and at the resource's next Deadliner deadline.
//  5. Otherwise: Poll.
//
// Every result is offset by a random ±JitterPercentage% of itself; for rule 4
// a deadline beyond Poll caps the jittered interval, so it is never overshot.
func NewHook(c Config) managed.PollIntervalHook {
	return newHook(c, time.Now, rand.Float64)
}

func newHook(c Config, now func() time.Time, rnd func() float64) managed.PollIntervalHook {
	jitter := func(d time.Duration) time.Duration {
		if c.JitterPercentage == 0 {
			return d
		}
		pct := float64(c.JitterPercentage)
		// Multiply before dividing so typical values stay exact in float64.
		return d + time.Duration(float64(d)*pct*(rnd()*2-1)/100)
	}

	var doubling, maxExp float64
	if c.AdaptivePollEnabled() {
		maxExp = math.Log2(float64(c.PollMax) / float64(c.Poll))
		// float64: PollMax barely above Poll makes this exceed int64.
		doubling = float64(c.PollMaxAfter) / maxExp
	}

	return func(mg resource.Managed, base time.Duration) time.Duration {
		if _, ok := meta.GetPollInterval(mg); ok {
			return jitter(base)
		}

		synced := mg.GetCondition(xpv2.TypeSynced)
		if synced.Status != corev1.ConditionTrue ||
			(synced.ObservedGeneration != 0 && synced.ObservedGeneration < mg.GetGeneration()) {
			return jitter(c.Poll)
		}

		t := now()
		ready := mg.GetCondition(xpv2.TypeReady)
		if ready.Reason == xpv2.ReasonCreating && t.Sub(ready.LastTransitionTime.Time) < creatingWindow {
			return jitter(min(creatingPollInterval, c.Poll))
		}

		if !c.AdaptivePollEnabled() || ready.Status != corev1.ConditionTrue {
			return jitter(c.Poll)
		}

		d := c.Poll
		if !ready.LastTransitionTime.IsZero() && !synced.LastTransitionTime.IsZero() {
			last := ready.LastTransitionTime.Time
			if synced.LastTransitionTime.After(last) {
				last = synced.LastTransitionTime.Time
			}
			if elapsed := t.Sub(last); elapsed > 0 {
				d = grow(c, doubling, maxExp, elapsed)
			}
		}

		if dl, ok := mg.(Deadliner); ok {
			if at, ok := dl.NextPollDeadline(); ok {
				until := at.Sub(t)
				if until <= c.Poll {
					return jitter(c.Poll)
				}
				return min(jitter(d), until)
			}
		}
		return jitter(d)
	}
}

// grow returns Poll·2^(elapsed/doubling), clamped to PollMax. Checking the
// exponent first keeps very long elapsed times from overflowing.
func grow(c Config, doubling, maxExp float64, elapsed time.Duration) time.Duration {
	x := float64(elapsed) / doubling
	if x >= maxExp {
		return c.PollMax
	}
	return min(time.Duration(float64(c.Poll)*math.Exp2(x)), c.PollMax)
}
