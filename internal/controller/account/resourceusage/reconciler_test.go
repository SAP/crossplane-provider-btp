package resourceusage

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/samber/lo"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
)

func TestReconciler(t *testing.T) {
	errBoom := errors.New("boom")

	type args struct {
		m manager.Manager
	}

	type want struct {
		result reconcile.Result
		err    error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"GetResourceUsageError": {
			reason: "Errors getting a resource usage should be returned",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(errBoom),
					},
					Scheme: fake.SchemeWith(&v1alpha1.ResourceUsage{}, &v1alpha1.ResourceUsageList{}),
				},
			},
			want: want{
				result: reconcile.Result{},
				err:    errors.Wrap(errBoom, errGetPC),
			},
		},
		"ResourceUsageNotFound": {
			reason: "We should return without requeueing if the resource usage no longer exists",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, "")),
					},
					Scheme: fake.SchemeWith(&v1alpha1.ResourceUsage{}, &v1alpha1.ResourceUsageList{}),
				},
			},
			want: want{
				result: reconcile.Result{},
				err:    nil,
			},
		},
		"TargetError": {
			reason: "We should return without requeueing if getting the target ends up in any error",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet:    test.NewMockGetFn(nil, GetResourceUsageAndTargetErrored),
						MockUpdate: test.NewMockUpdateFn(nil),
						MockDelete: test.NewMockDeleteFn(errBoom),
					},
					Scheme: fake.SchemeWith(&v1alpha1.ResourceUsage{}, &v1alpha1.ResourceUsageList{}),
				},
			},
			want: want{
				result: reconcile.Result{},
				err:    errBoom,
			},
		},
		"DeleteResourceUsageError": {
			reason: "We should requeue after a short wait if we encounter an error deleting a resource usage",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet:    test.NewMockGetFn(nil, GetResourceUsageAndNotExistingTarget),
						MockUpdate: test.NewMockUpdateFn(nil),
						MockDelete: test.NewMockDeleteFn(errBoom),
					},
					Scheme: fake.SchemeWith(&v1alpha1.ResourceUsage{}, &v1alpha1.ResourceUsageList{}),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"BlockDeleteWhileInUse": {
			reason: "We should return without requeueing if the resource usage is still in use",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet:          test.NewMockGetFn(nil, GetDeletedResourceUsageAndTarget),
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
						MockUpdate:       test.NewMockUpdateFn(nil),
						MockDelete:       test.NewMockDeleteFn(nil),
					},
					Scheme: fake.SchemeWith(&v1alpha1.ResourceUsage{}, &v1alpha1.ResourceUsageList{}),
				},
			},
			want: want{
				result: reconcile.Result{Requeue: false},
			},
		},
		"RemoveFinalizerError": {
			reason: "We should requeue after a short wait if we encounter an error while removing our finalizer",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(
							nil, GetDeletedResourceUsageAndNotExistingTarget,
						),
						MockUpdate: test.NewMockUpdateFn(errBoom),
					},
					Scheme: fake.SchemeWith(&v1alpha1.ResourceUsage{}, &v1alpha1.ResourceUsageList{}),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"SuccessfulDelete": {
			reason: "We should return without requeueing when we successfully remove our finalizer",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet:    test.NewMockGetFn(nil, GetDeletedResourceUsageAndNotExistingTarget),
						MockUpdate: test.NewMockUpdateFn(nil),
						MockDelete: test.NewMockDeleteFn(
							nil, func(obj client.Object) error {
								_, ok := obj.(*v1alpha1.ResourceUsage)
								if ok {
									return nil
								}
								return errBoom
							},
						),
					},
					Scheme: fake.SchemeWith(&v1alpha1.ResourceUsage{}, &v1alpha1.ResourceUsageList{}),
				},
			},
			want: want{
				result: reconcile.Result{Requeue: false},
			},
		},
		"AddFinalizerError": {
			reason: "We should requeue after a short wait if we encounter an error while adding our finalizer",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet:    test.NewMockGetFn(nil, GetResourceUsageAndTarget),
						MockUpdate: test.NewMockUpdateFn(errBoom),
					},
					Scheme: fake.SchemeWith(&v1alpha1.ResourceUsage{}, &v1alpha1.ResourceUsageList{}),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"FinalizerExists": {
			reason: "We should not requeue if our finalizer exists",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet:    test.NewMockGetFn(nil, GetResourceWithFinalizerUsageAndTarget),
						MockUpdate: test.NewMockUpdateFn(errBoom),
					},
					Scheme: fake.SchemeWith(&v1alpha1.ResourceUsage{}, &v1alpha1.ResourceUsageList{}),
				},
			},
			want: want{
				result: reconcile.Result{Requeue: false},
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {
				r := NewReconciler(tc.args.m)
				got, err := r.Reconcile(context.Background(), reconcile.Request{})

				if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\nr.Reconcile(...): -want error, +got error:\n%s", tc.reason, diff)
				}

				if diff := cmp.Diff(tc.want.result, got); diff != "" {
					t.Errorf("\n%s\nr.Reconcile(...): -want, +got:\n%s", tc.reason, diff)
				}
			},
		)
	}
}

func GetDeletedResourceUsageAndNotExistingTarget(obj client.Object) error {
	ru, isRu := obj.(*v1alpha1.ResourceUsage)
	partial, isPartial := obj.(*metav1.PartialObjectMetadata)
	if isPartial {
		return kerrors.NewNotFound(schema.GroupResource{}, partial.Name)
	}
	if isRu {
		ru.SetDeletionTimestamp(lo.ToPtr(metav1.Now()))
		return nil
	}
	panic("unhandled resource")
}

func GetResourceUsageAndNotExistingTarget(obj client.Object) error {
	ru, isRu := obj.(*v1alpha1.ResourceUsage)
	partial, isPartial := obj.(*metav1.PartialObjectMetadata)
	if isPartial {
		return kerrors.NewNotFound(schema.GroupResource{}, partial.Name)
	}
	if isRu {
		ru.Name = "fake"
		return nil
	}
	panic("unhandled resource")
}

func GetDeletedResourceUsageAndTarget(obj client.Object) error {
	ru, isRu := obj.(*v1alpha1.ResourceUsage)
	partial, isPartial := obj.(*metav1.PartialObjectMetadata)
	if isPartial {
		partial.Name = "fake"
		return nil
	}
	if isRu {
		ru.Name = "fake"
		ru.SetDeletionTimestamp(lo.ToPtr(metav1.Now()))
		return nil
	}
	panic("unhandled resource")
}

func GetResourceUsageAndTarget(obj client.Object) error {
	ru, isRu := obj.(*v1alpha1.ResourceUsage)
	partial, isPartial := obj.(*metav1.PartialObjectMetadata)
	if isPartial {
		partial.Name = "fake"
		return nil
	}
	if isRu {
		ru.Name = "fake"
		return nil
	}
	panic("unhandled resource")
}

func GetResourceUsageAndTargetErrored(obj client.Object) error {
	ru, isRu := obj.(*v1alpha1.ResourceUsage)
	_, isPartial := obj.(*metav1.PartialObjectMetadata)
	if isPartial {
		return errors.New("boom")
	}
	if isRu {
		ru.Name = "fake"
		return nil
	}
	panic("unhandled resource")
}

func GetResourceWithFinalizerUsageAndTarget(obj client.Object) error {
	ru, isRu := obj.(*v1alpha1.ResourceUsage)
	partial, isPartial := obj.(*metav1.PartialObjectMetadata)
	if isPartial {
		partial.Name = "fake"
		return nil
	}
	if isRu {
		ru.Name = "fake"
		ru.Finalizers = append(ru.Finalizers, v1alpha1.Finalizer)
		return nil
	}
	panic("unhandled resource")
}
