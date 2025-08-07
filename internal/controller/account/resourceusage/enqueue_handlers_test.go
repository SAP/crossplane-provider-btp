package resourceusage

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
)

var (
	_ handler.EventHandler = &EnqueueRequestForResourceUsage{}
)

type addFn func(item reconcile.Request)

func (fn addFn) Add(item reconcile.Request) {
	fn(item)
}

func TestAddResourceUsage(t *testing.T) {
	name := "coolname"

	cases := map[string]struct {
		obj   runtime.Object
		queue adder
	}{
		"NotResourceUsageReferencer": {
			queue: addFn(func(got reconcile.Request) { t.Errorf("queue.Add() called unexpectedly") }),
		},
		"IsResourceUsageReferencer": {
			obj: &v1alpha1.ResourceUsage{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
			},
			queue: addFn(
				func(got reconcile.Request) {
					want := reconcile.Request{NamespacedName: types.NamespacedName{Name: name}}
					if diff := cmp.Diff(want, got); diff != "" {
						t.Errorf("-want, +got:\n%s", diff)
					}
				},
			),
		},
	}

	for _, tc := range cases {
		addResourceUsage(tc.obj, tc.queue)
	}
}
