package pollinterval

import (
	"math"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var t0 = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func nowT0() time.Time { return t0 }

func mr(gen int64, ann string, conds ...xpv2.Condition) *fake.LegacyManaged {
	m := &fake.LegacyManaged{}
	m.SetGeneration(gen)
	if ann != "" {
		meta.AddAnnotations(m, map[string]string{meta.AnnotationKeyPollInterval: ann})
	}
	m.SetConditions(conds...)
	return m
}

func cond(t xpv2.ConditionType, s corev1.ConditionStatus, r xpv2.ConditionReason, ago time.Duration, og int64) xpv2.Condition {
	return xpv2.Condition{Type: t, Status: s, Reason: r, LastTransitionTime: metav1.NewTime(t0.Add(-ago)), ObservedGeneration: og}
}

func synced(ago time.Duration) xpv2.Condition {
	return cond(xpv2.TypeSynced, corev1.ConditionTrue, xpv2.ReasonReconcileSuccess, ago, 1)
}

func ready(ago time.Duration) xpv2.Condition {
	return cond(xpv2.TypeReady, corev1.ConditionTrue, xpv2.ReasonAvailable, ago, 0)
}

type deadlined struct {
	*fake.LegacyManaged
	at time.Time
	ok bool
}

func (d deadlined) NextPollDeadline() (time.Time, bool) { return d.at, d.ok }

// cfg yields T = 30m / log2(8) = 10m exactly.
var cfg = Config{Poll: time.Minute, PollMax: 8 * time.Minute, PollMaxAfter: 30 * time.Minute}

func TestRules(t *testing.T) {
	zeroReady := ready(0)
	zeroReady.LastTransitionTime = metav1.Time{}
	stable := func() *fake.LegacyManaged { return mr(1, "", ready(45*time.Minute), synced(45*time.Minute)) }

	cases := map[string]struct {
		cfg  *Config
		mg   resource.Managed
		base time.Duration
		want time.Duration
	}{
		"AnnotationWins": {
			mg:   mr(1, "5m", cond(xpv2.TypeReady, corev1.ConditionFalse, xpv2.ReasonCreating, time.Minute, 0), synced(time.Hour)),
			base: 5 * time.Minute, want: 5 * time.Minute,
		},
		"AnnotationWinsOverDeadline": {
			mg:   deadlined{mr(1, "10m", ready(time.Hour), synced(time.Hour)), t0.Add(time.Minute), true},
			base: 10 * time.Minute, want: 10 * time.Minute,
		},
		"NotSyncedUsesPoll": {
			mg:   mr(1, "", ready(time.Hour), cond(xpv2.TypeSynced, corev1.ConditionFalse, xpv2.ReasonReconcileError, time.Hour, 1)),
			want: time.Minute,
		},
		"SyncedBehindGenerationUsesPoll": {
			mg:   mr(3, "", ready(time.Hour), cond(xpv2.TypeSynced, corev1.ConditionTrue, xpv2.ReasonReconcileSuccess, time.Hour, 2)),
			want: time.Minute,
		},
		"ZeroObservedGenerationNotBehind": {
			mg:   mr(3, "", ready(time.Hour), cond(xpv2.TypeSynced, corev1.ConditionTrue, xpv2.ReasonReconcileSuccess, time.Hour, 0)),
			want: 8 * time.Minute,
		},
		"CreatingInWindow": {
			mg:   mr(1, "", synced(time.Hour), cond(xpv2.TypeReady, corev1.ConditionFalse, xpv2.ReasonCreating, 9*time.Minute+59*time.Second, 0)),
			want: 15 * time.Second,
		},
		"CreatingPastWindow": {
			mg:   mr(1, "", synced(time.Hour), cond(xpv2.TypeReady, corev1.ConditionFalse, xpv2.ReasonCreating, 10*time.Minute, 0)),
			want: time.Minute,
		},
		"AdoptionNeverSlowerThanPoll": {
			cfg:  &Config{Poll: 10 * time.Second, PollMax: 8 * time.Minute, PollMaxAfter: 30 * time.Minute},
			mg:   mr(1, "", synced(time.Hour), cond(xpv2.TypeReady, corev1.ConditionFalse, xpv2.ReasonCreating, time.Minute, 0)),
			base: 10 * time.Second, want: 10 * time.Second,
		},
		"NotReadyUsesPoll": {
			mg:   mr(1, "", synced(time.Hour), cond(xpv2.TypeReady, corev1.ConditionFalse, xpv2.ReasonUnavailable, time.Hour, 0)),
			want: time.Minute,
		},
		"BackoffOffUsesPoll": {
			cfg:  &Config{Poll: time.Minute},
			mg:   mr(1, "", ready(time.Hour), synced(time.Hour)),
			want: time.Minute,
		},
		"PollMaxEqualsPollUsesPoll": {
			cfg:  &Config{Poll: time.Minute, PollMax: time.Minute, PollMaxAfter: 30 * time.Minute},
			mg:   mr(1, "", ready(time.Hour), synced(time.Hour)),
			want: time.Minute,
		},
		"GrowthStartsAtPoll":         {mg: mr(1, "", ready(0), synced(0)), want: time.Minute},
		"GrowthOneDoubling":          {mg: mr(1, "", ready(time.Hour), synced(10*time.Minute)), want: 2 * time.Minute},
		"GrowthTwoDoublings":         {mg: mr(1, "", ready(20*time.Minute), synced(20*time.Minute)), want: 4 * time.Minute},
		"GrowthCapped":               {mg: stable(), want: 8 * time.Minute},
		"ZeroTransitionTimeUsesPoll": {mg: mr(1, "", zeroReady, synced(time.Hour)), want: time.Minute},
		"FutureTransitionUsesPoll":   {mg: mr(1, "", ready(-5*time.Minute), synced(time.Hour)), want: time.Minute},
		"OverflowClampsToPollMax": {
			cfg:  &Config{Poll: time.Second, PollMax: 24 * time.Hour, PollMaxAfter: time.Second},
			mg:   mr(1, "", ready(8760*time.Hour), synced(8760*time.Hour)),
			base: time.Second, want: 24 * time.Hour,
		},
		"DeadlineCapsBackoff":           {mg: deadlined{stable(), t0.Add(3 * time.Minute), true}, want: 3 * time.Minute},
		"DeadlineWithinPollUsesPoll":    {mg: deadlined{stable(), t0.Add(20 * time.Second), true}, want: time.Minute},
		"DeadlinePastUsesPoll":          {mg: deadlined{stable(), t0.Add(-time.Minute), true}, want: time.Minute},
		"DeadlineBeyondBackoffNoEffect": {mg: deadlined{stable(), t0.Add(time.Hour), true}, want: 8 * time.Minute},
		"NoDeadlineReported":            {mg: deadlined{stable(), time.Time{}, false}, want: 8 * time.Minute},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := cfg
			if tc.cfg != nil {
				c = *tc.cfg
			}
			base := tc.base
			if base == 0 {
				base = c.Poll
			}
			hook := newHook(c, nowT0, func() float64 { return 0.5 })
			if got := hook(tc.mg, base); got != tc.want {
				t.Errorf("hook() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIssueCurve(t *testing.T) {
	c := Config{Poll: time.Minute, PollMax: 10 * time.Minute, PollMaxAfter: 30 * time.Minute}
	tc := time.Duration(float64(30*time.Minute) / math.Log2(10))
	hook := newHook(c, nowT0, func() float64 { return 0.5 })
	cases := []struct {
		elapsed, want time.Duration
	}{
		{0, time.Minute},
		{tc, 2 * time.Minute},
		{2 * tc, 4 * time.Minute},
		{3 * tc, 8 * time.Minute},
		{30 * time.Minute, 10 * time.Minute},
		{2 * time.Hour, 10 * time.Minute},
	}
	for _, cs := range cases {
		got := hook(mr(1, "", ready(cs.elapsed), synced(cs.elapsed)), time.Minute)
		if d := got - cs.want; d < -time.Second || d > time.Second {
			t.Errorf("elapsed %v: hook() = %v, want %v ±1s", cs.elapsed, got, cs.want)
		}
	}
}

func TestJitterThenCap(t *testing.T) {
	c := cfg
	c.JitterPercentage = 10
	hook := newHook(c, nowT0, func() float64 { return math.Nextafter(1, 0) })
	stable := mr(1, "", ready(45*time.Minute), synced(45*time.Minute))

	if got := hook(stable, time.Minute); got <= 8*time.Minute || got >= 8*time.Minute+48*time.Second {
		t.Errorf("no deadline: hook() = %v, want in (8m, 8m48s)", got)
	}
	if got := hook(deadlined{stable, t0.Add(3 * time.Minute), true}, time.Minute); got != 3*time.Minute {
		t.Errorf("deadline: hook() = %v, want 3m", got)
	}
}

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
			hook := newHook(Config{Poll: time.Minute, JitterPercentage: tc.pct}, nowT0, func() float64 { return tc.rnd })
			if got := hook(mr(1, tc.base.String()), tc.base); got != tc.want {
				t.Errorf("hook(%v) with pct=%d rnd=%v = %v, want %v", tc.base, tc.pct, tc.rnd, got, tc.want)
			}
		})
	}
}

func TestHookStaysWithinBand(t *testing.T) {
	const pct = 10
	hook := NewHook(Config{Poll: time.Minute, JitterPercentage: pct})
	base := time.Minute
	lo, hi := base*(100-pct)/100, base*(100+pct)/100
	for range 1000 {
		if got := hook(mr(1, ""), base); got < lo || got >= hi {
			t.Fatalf("hook(%v) = %v, want in [%v, %v)", base, got, lo, hi)
		}
	}
}

func TestRealTypeCapsBackoff(t *testing.T) {
	sb := &accountv1alpha1.ServiceBinding{}
	sb.SetGeneration(1)
	sb.SetConditions(ready(45*time.Minute), synced(45*time.Minute))
	sb.Spec.Rotation = &accountv1alpha1.RotationParameters{Frequency: &providerv1alpha1.Duration{Duration: time.Hour}}
	sb.Status.AtProvider.CreatedDate = &metav1.Time{Time: t0.Add(-57 * time.Minute)}

	hook := newHook(cfg, nowT0, func() float64 { return 0.5 })
	if got := hook(sb, time.Minute); got != 3*time.Minute {
		t.Errorf("hook() = %v, want 3m", got)
	}
}
