package externalstate

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/prometheus/client_golang/prometheus/testutil"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
)

func scheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("cannot build scheme: %v", err)
	}
	return s
}

func serviceInstance(name, state string) *v1alpha1.ServiceInstance {
	si := &v1alpha1.ServiceInstance{}
	si.SetName(name)
	si.Status.AtProvider.State = state
	return si
}

func entitlement(name, entityState string) *v1alpha1.Entitlement {
	e := &v1alpha1.Entitlement{}
	e.SetName(name)
	e.Status.AtProvider = &v1alpha1.EntitlementObservation{
		Assigned: &v1alpha1.Assignable{EntityState: entityState},
	}
	return e
}

func subaccount(name, state string) *v1alpha1.Subaccount {
	sa := &v1alpha1.Subaccount{}
	sa.SetName(name)
	sa.Status.AtProvider.Status = internal.Ptr(state)
	return sa
}

func metaWithName(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name}
}

func newCollector(c client.Reader) *collector {
	return &collector{reader: c, log: logging.NewNopLogger()}
}

func TestCollect(t *testing.T) {
	gauge.Reset()
	t.Cleanup(gauge.Reset)

	kube := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(
		serviceInstance("si-a", "failed"),
		serviceInstance("si-b", "failed"),
		serviceInstance("si-c", "succeeded"),
		entitlement("ent-a", "PROCESSING_FAILED"),
		subaccount("sa-a", "OK"),
	).Build()

	newCollector(kube).collect(context.Background())

	want := `
# HELP btp_managed_resource_external_state Number of managed resources per managed kind and external (atProvider) state.
# TYPE btp_managed_resource_external_state gauge
btp_managed_resource_external_state{kind="Entitlement",state="PROCESSING_FAILED"} 1
btp_managed_resource_external_state{kind="ServiceInstance",state="failed"} 2
btp_managed_resource_external_state{kind="ServiceInstance",state="succeeded"} 1
btp_managed_resource_external_state{kind="Subaccount",state="OK"} 1
`
	if err := testutil.CollectAndCompare(gauge, strings.NewReader(want)); err != nil {
		t.Errorf("unexpected metrics: %v", err)
	}
}

// TestCollectEmptyStateIsUnknown pins that a resource whose external state has
// not been observed yet lands in a single bounded bucket instead of minting an
// empty-label series.
func TestCollectEmptyStateIsUnknown(t *testing.T) {
	gauge.Reset()
	t.Cleanup(gauge.Reset)

	kube := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(
		serviceInstance("si-a", ""),
		// An Entitlement with no observation at all must be nil-safe.
		&v1alpha1.Entitlement{ObjectMeta: metaWithName("ent-a")},
		// A Subaccount whose status pointer is nil must be nil-safe too.
		&v1alpha1.Subaccount{ObjectMeta: metaWithName("sa-a")},
	).Build()

	newCollector(kube).collect(context.Background())

	for _, kind := range []string{"ServiceInstance", "Entitlement", "Subaccount"} {
		if got := testutil.ToFloat64(gauge.WithLabelValues(kind, unknownState)); got != 1 {
			t.Errorf("expected 1 %s in state %q, got %v", kind, unknownState, got)
		}
	}
}

// TestCollectResetDropsStaleSeries pins that a state series disappears once no
// resource is in that state any more, instead of lingering at its last value.
func TestCollectResetDropsStaleSeries(t *testing.T) {
	gauge.Reset()
	t.Cleanup(gauge.Reset)

	failed := serviceInstance("si-a", "failed")
	kube := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(
		failed,
		serviceInstance("si-b", "succeeded"),
	).Build()

	c := newCollector(kube)
	c.collect(context.Background())

	if got := testutil.ToFloat64(gauge.WithLabelValues("ServiceInstance", "failed")); got != 1 {
		t.Fatalf("expected 1 failed ServiceInstance, got %v", got)
	}

	if err := kube.Delete(context.Background(), failed); err != nil {
		t.Fatalf("cannot delete: %v", err)
	}
	c.collect(context.Background())

	want := `
# HELP btp_managed_resource_external_state Number of managed resources per managed kind and external (atProvider) state.
# TYPE btp_managed_resource_external_state gauge
btp_managed_resource_external_state{kind="ServiceInstance",state="succeeded"} 1
`
	if err := testutil.CollectAndCompare(gauge, strings.NewReader(want),
		"btp_managed_resource_external_state"); err != nil {
		t.Errorf("expected the failed series to be dropped: %v", err)
	}
}

// TestCollectSkipsUnlistableKinds pins that a kind whose CRD is absent - the
// normal case on a trimmed installation where controllers are disabled - is
// logged and skipped rather than taking the collector, and with it the
// provider, down.
func TestCollectSkipsUnlistableKinds(t *testing.T) {
	gauge.Reset()
	t.Cleanup(gauge.Reset)

	kube := fake.NewClientBuilder().WithScheme(scheme(t)).
		WithObjects(subaccount("sa-a", "OK")).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*v1alpha1.ServiceInstanceList); ok {
					return &apimeta.NoKindMatchError{}
				}
				return c.List(ctx, list, opts...)
			},
		}).Build()

	newCollector(kube).collect(context.Background())

	if got := testutil.ToFloat64(gauge.WithLabelValues("Subaccount", "OK")); got != 1 {
		t.Errorf("expected the remaining kinds to still be collected, got %v", got)
	}
}

// TestCollectSurvivesAWedgedList pins the bound on a single List. Listing
// through the manager's cache starts an informer and waits for it to sync; an
// informer that can never sync (list/watch denied, stalled initial LIST) would
// otherwise block collect() until the manager stops - the ticker would never
// fire again and the gauge would freeze at its last value with nothing
// reporting why. The wedged kind must degrade into a skipped kind.
func TestCollectSurvivesAWedgedList(t *testing.T) {
	gauge.Reset()
	t.Cleanup(gauge.Reset)

	kube := fake.NewClientBuilder().WithScheme(scheme(t)).
		WithObjects(subaccount("sa-a", "OK")).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*v1alpha1.ServiceInstanceList); ok {
					// Never returns on its own: only the bound can end this.
					<-ctx.Done()
					return ctx.Err()
				}
				return c.List(ctx, list, opts...)
			},
		}).Build()

	c := newCollector(kube)
	c.listTimeout = 10 * time.Millisecond

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.collect(context.Background())
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("collect() blocked on a List that never returns")
	}

	if got := testutil.ToFloat64(gauge.WithLabelValues("Subaccount", "OK")); got != 1 {
		t.Errorf("expected the remaining kinds to still be collected, got %v", got)
	}
}

func TestNormalizeState(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"Empty":     {in: "", want: unknownState},
		"Known":     {in: "PROCESSING_FAILED", want: "PROCESSING_FAILED"},
		"WithSpace": {in: "in progress", want: "in progress"},
		"Overlong":  {in: strings.Repeat("x", 200), want: strings.Repeat("x", maxStateLabelBytes)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := normalizeState(tc.in); got != tc.want {
				t.Errorf("normalizeState(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
