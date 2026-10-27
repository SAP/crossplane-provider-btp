package options

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
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

// newPollReconciler returns a reconciler for an up-to-date resource that
// carries the given poll-interval annotation ("" for none), with --poll=1m.
func newPollReconciler(ann string, extra []managed.ReconcilerOption) *managed.Reconciler {
	m := &fake.Manager{
		Client: &test.MockClient{
			MockGet: func(_ context.Context, _ client.ObjectKey, obj client.Object) error {
				if ann != "" {
					obj.SetAnnotations(map[string]string{meta.AnnotationKeyPollInterval: ann})
				}
				return nil
			},
			MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
		},
		Scheme: fake.SchemeWith(&fake.LegacyManaged{}),
	}
	opts := append([]managed.ReconcilerOption{
		managed.WithInitializers(),
		managed.WithReferenceResolver(managed.ReferenceResolverFn(func(context.Context, resource.Managed) error { return nil })),
		managed.WithExternalConnector(managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) {
			return &managed.ExternalClientFns{
				ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
					return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
				},
				DisconnectFn: func(context.Context) error { return nil },
			}, nil
		})),
		managed.WithFinalizer(resource.FinalizerFns{AddFinalizerFn: func(context.Context, resource.Object) error { return nil }}),
		managed.WithPollInterval(time.Minute),
	}, extra...)
	return managed.NewReconciler(m, resource.ManagedKind(fake.GVK(&fake.LegacyManaged{})), opts...)
}

func TestPollReconcilerOptions(t *testing.T) {
	structs := map[string]func(pct uint) []managed.ReconcilerOption{
		"CrossplaneOptions": func(pct uint) []managed.ReconcilerOption {
			return CrossplaneOptions{PollJitterPercentage: pct}.PollReconcilerOptions()
		},
		"UpjetOptions": func(pct uint) []managed.ReconcilerOption {
			return UpjetOptions{PollJitterPercentage: pct}.PollReconcilerOptions()
		},
	}
	cases := map[string]struct {
		pct  uint
		ann  string
		base time.Duration // expected effective interval before jitter
	}{
		"NoJitterUsesPoll":              {pct: 0, ann: "", base: time.Minute},
		"NoJitterFloorsShortAnnotation": {pct: 0, ann: "100ms", base: time.Second},
		"NoJitterUsesLongAnnotation":    {pct: 0, ann: "10m", base: 10 * time.Minute},
		"JitterAroundPoll":              {pct: 10, ann: "", base: time.Minute},
		"JitterAroundFlooredAnnotation": {pct: 10, ann: "100ms", base: time.Second},
		"JitterAroundLongAnnotation":    {pct: 10, ann: "10m", base: 10 * time.Minute},
	}
	const runs = 20
	for sname, optsFor := range structs {
		for name, tc := range cases {
			t.Run(sname+"/"+name, func(t *testing.T) {
				r := newPollReconciler(tc.ann, optsFor(tc.pct))
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
