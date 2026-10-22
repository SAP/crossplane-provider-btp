package options

import (
	"context"
	"testing"
	"time"

	xpcontroller "github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	tjcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestWithBackoffDisablesPriorityQueue(t *testing.T) {
	co := CrossplaneOptions{}
	if got := co.ForControllerRuntimeWithBackoff().UsePriorityQueue; got == nil || *got != false {
		t.Errorf("CrossplaneOptions: UsePriorityQueue = %v, want %v", got, ptr.To(false))
	}
	uo := UpjetOptions{}
	if got := uo.ForControllerRuntimeWithBackoff().UsePriorityQueue; got == nil || *got != false {
		t.Errorf("UpjetOptions: UsePriorityQueue = %v, want %v", got, ptr.To(false))
	}
}

// newPollReconciler returns a reconciler for an existing resource that
// carries the given poll-interval annotation ("" for none), with --poll=1m.
// setup (optional) mutates the resource on every Get; onStatus (optional)
// sees every status update. The default external client reports the
// resource up to date; extra options may replace it.
func newPollReconciler(ann string, setup func(*fake.LegacyManaged), onStatus func(client.Object), extra []managed.ReconcilerOption) *managed.Reconciler {
	m := &fake.Manager{
		Client: &test.MockClient{
			MockGet: func(_ context.Context, _ client.ObjectKey, obj client.Object) error {
				if ann != "" {
					obj.SetAnnotations(map[string]string{meta.AnnotationKeyPollInterval: ann})
				}
				if setup != nil {
					setup(obj.(*fake.LegacyManaged))
				}
				return nil
			},
			MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil, func(obj client.Object) error {
				if onStatus != nil {
					onStatus(obj)
				}
				return nil
			}),
		},
		Scheme: fake.SchemeWith(&fake.LegacyManaged{}),
	}
	opts := append([]managed.ReconcilerOption{
		managed.WithInitializers(),
		managed.WithReferenceResolver(managed.ReferenceResolverFn(func(context.Context, resource.Managed) error { return nil })),
		managed.WithExternalConnector(externalConnector(true)),
		managed.WithFinalizer(resource.FinalizerFns{AddFinalizerFn: func(context.Context, resource.Object) error { return nil }}),
		managed.WithPollInterval(time.Minute),
	}, extra...)
	return managed.NewReconciler(m, resource.ManagedKind(fake.GVK(&fake.LegacyManaged{})), opts...)
}

// externalConnector connects a client whose Observe reports an existing
// resource and whose Update succeeds.
func externalConnector(upToDate bool) managed.ExternalConnector {
	return managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) {
		return &managed.ExternalClientFns{
			ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
				return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
			},
			UpdateFn: func(context.Context, resource.Managed) (managed.ExternalUpdate, error) {
				return managed.ExternalUpdate{}, nil
			},
			DisconnectFn: func(context.Context) error { return nil },
		}, nil
	})
}

// stableSince marks the resource Ready and Synced as of ago.
func stableSince(ago time.Duration) func(*fake.LegacyManaged) {
	return func(mg *fake.LegacyManaged) {
		ltt := metav1.NewTime(time.Now().Add(-ago))
		r, s := xpv2.Available(), xpv2.ReconcileSuccess()
		r.LastTransitionTime, s.LastTransitionTime = ltt, ltt
		mg.SetConditions(r, s)
	}
}

type optionsStruct interface {
	PollReconcilerOptions() []managed.ReconcilerOption
	WithExternalConnector(managed.ExternalConnector) managed.ReconcilerOption
}

// optionStructs builds both option structs with --poll=1m.
var optionStructs = map[string]func(pct uint, pollMax, pollMaxAfter time.Duration) optionsStruct{
	"CrossplaneOptions": func(pct uint, pollMax, pollMaxAfter time.Duration) optionsStruct {
		return CrossplaneOptions{
			Options:              xpcontroller.Options{PollInterval: time.Minute},
			PollJitterPercentage: pct, PollMax: pollMax, PollMaxAfter: pollMaxAfter,
		}
	},
	"UpjetOptions": func(pct uint, pollMax, pollMaxAfter time.Duration) optionsStruct {
		return UpjetOptions{
			Options:              tjcontroller.Options{Options: xpcontroller.Options{PollInterval: time.Minute}},
			PollJitterPercentage: pct, PollMax: pollMax, PollMaxAfter: pollMaxAfter,
		}
	},
}

func TestPollReconcilerOptions(t *testing.T) {
	cases := map[string]struct {
		pct     uint
		ann     string
		pollMax time.Duration
		setup   func(*fake.LegacyManaged)
		base    time.Duration // expected effective interval before jitter
	}{
		"NoJitterUsesPoll":              {pct: 0, ann: "", base: time.Minute},
		"NoJitterFloorsShortAnnotation": {pct: 0, ann: "100ms", base: time.Second},
		"NoJitterUsesLongAnnotation":    {pct: 0, ann: "10m", base: 10 * time.Minute},
		"JitterAroundPoll":              {pct: 10, ann: "", base: time.Minute},
		"JitterAroundFlooredAnnotation": {pct: 10, ann: "100ms", base: time.Second},
		"JitterAroundLongAnnotation":    {pct: 10, ann: "10m", base: 10 * time.Minute},
		"StableBacksOffToPollMax":       {pct: 0, pollMax: 8 * time.Minute, setup: stableSince(45 * time.Minute), base: 8 * time.Minute},
	}
	const runs = 20
	for sname, build := range optionStructs {
		for name, tc := range cases {
			t.Run(sname+"/"+name, func(t *testing.T) {
				o := build(tc.pct, tc.pollMax, 30*time.Minute)
				r := newPollReconciler(tc.ann, tc.setup, nil, o.PollReconcilerOptions())
				lo := tc.base * time.Duration(100-tc.pct) / 100
				hi := tc.base * time.Duration(100+tc.pct) / 100
				seen := map[time.Duration]bool{}
				for range runs {
					res, err := r.Reconcile(context.Background(), reconcile.Request{})
					if err != nil {
						t.Fatalf("Reconcile() error = %v", err)
					}
					got := res.RequeueAfter
					if got < lo || got > hi {
						t.Fatalf("RequeueAfter = %v, want in [%v, %v]", got, lo, hi)
					}
					seen[got] = true
				}
				// The band contains base, so a band check alone would also pass
				// if the hook were missing or ignored the percentage.
				if tc.pct == 0 && len(seen) != 1 {
					t.Errorf("pct=0: got %d distinct RequeueAfter values, want exactly 1", len(seen))
				}
				if tc.pct > 0 && len(seen) < 2 {
					t.Errorf("pct=%d: all %d RequeueAfter values were identical; jitter not applied", tc.pct, runs)
				}
			})
		}
	}
}

// TestWithExternalConnectorDriftReset runs a full reconcile that corrects
// drift with Update, so it also proves the refreshed Synced time survives the
// runtime's own ReconcileSuccess marking.
func TestWithExternalConnectorDriftReset(t *testing.T) {
	cases := map[string]struct {
		pollMax   time.Duration
		refreshed bool
	}{
		"BackoffOffKeepsSynced":    {pollMax: 0},
		"BackoffOnRefreshesSynced": {pollMax: 8 * time.Minute, refreshed: true},
	}
	for sname, build := range optionStructs {
		for name, tc := range cases {
			t.Run(sname+"/"+name, func(t *testing.T) {
				o := build(0, tc.pollMax, 30*time.Minute)
				var got xpv2.Condition
				onStatus := func(obj client.Object) { got = obj.(*fake.LegacyManaged).GetCondition(xpv2.TypeSynced) }
				extra := append(o.PollReconcilerOptions(), o.WithExternalConnector(externalConnector(false)))
				r := newPollReconciler("", stableSince(time.Hour), onStatus, extra)

				start := time.Now().Add(-time.Second)
				if _, err := r.Reconcile(context.Background(), reconcile.Request{}); err != nil {
					t.Fatalf("Reconcile() error = %v", err)
				}
				if got.Reason != xpv2.ReasonReconcileSuccess {
					t.Fatalf("Synced reason = %q, want %q (Update path not taken?)", got.Reason, xpv2.ReasonReconcileSuccess)
				}
				if refreshed := got.LastTransitionTime.After(start); refreshed != tc.refreshed {
					t.Errorf("Synced LTT = %v; refreshed = %v, want %v", got.LastTransitionTime, refreshed, tc.refreshed)
				}
			})
		}
	}
}
