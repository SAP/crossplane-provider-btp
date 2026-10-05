package resourceusage

import (
	"context"
	"fmt"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	resourcefake "github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

func TestReconcileTargetIdentity(t *testing.T) {
	errForbidden := kerrors.NewForbidden(schema.GroupResource{Group: accountv1alpha1.ServiceBindingGroupVersionKind.Group, Resource: "servicebindings"}, "dependent", fmt.Errorf("denied"))
	tests := map[string]struct {
		label          string
		missingLabel   bool
		targetUID      string
		missingTarget  bool
		targetDeleting bool
		deleting       bool
		getError       error
		wantDeleted    bool
	}{
		"replacement":                      {label: "old-target-uid", targetUID: "new-target-uid", deleting: true, wantDeleted: true},
		"same target":                      {label: "old-target-uid", targetUID: "old-target-uid", deleting: true},
		"same target still deleting":       {label: "old-target-uid", targetUID: "old-target-uid", deleting: true, targetDeleting: true},
		"missing label":                    {missingLabel: true, targetUID: "new-target-uid", deleting: true},
		"empty label":                      {targetUID: "new-target-uid", deleting: true},
		"invalid label":                    {label: "invalid/uid", targetUID: "new-target-uid", deleting: true},
		"unknown current UID":              {label: "old-target-uid", deleting: true},
		"missing target":                   {label: "old-target-uid", missingTarget: true, deleting: true, wantDeleted: true},
		"target lookup forbidden":          {label: "old-target-uid", targetUID: "new-target-uid", deleting: true, getError: errForbidden},
		"active usage with replacement":    {label: "old-target-uid", targetUID: "new-target-uid"},
		"active usage with missing target": {label: "old-target-uid", missingTarget: true, wantDeleted: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			scheme := runtime.NewScheme()
			if err := v1alpha1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			if err := accountv1alpha1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			target := &accountv1alpha1.ServiceBinding{ObjectMeta: metav1.ObjectMeta{Name: "dependent"}}
			target.SetUID(types.UID(tc.targetUID))
			target.SetGroupVersionKind(accountv1alpha1.ServiceBindingGroupVersionKind)
			if tc.targetDeleting {
				target.SetDeletionTimestamp(&metav1.Time{Time: metav1.Now().Time})
				target.SetFinalizers([]string{"test/target-finalizer"})
			}
			ru := targetIdentityUsage(target)
			ru.Labels[v1alpha1.LabelKeyTargetUid] = tc.label
			if tc.missingLabel {
				ru.Labels = nil
			}
			if tc.deleting {
				now := metav1.Now()
				ru.SetDeletionTimestamp(&now)
			}
			objects := []client.Object{ru}
			if !tc.missingTarget {
				objects = append(objects, target)
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*metav1.PartialObjectMetadata); ok && tc.getError != nil {
						return tc.getError
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}).Build()
			r := NewReconciler(&resourcefake.Manager{Client: c, Scheme: scheme})
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(ru)})
			if err != tc.getError {
				t.Fatalf("Reconcile() error = %v, want %v", err, tc.getError)
			}
			if result != (reconcile.Result{}) {
				t.Fatalf("Reconcile() result = %v, want no requeue", result)
			}
			got := &v1alpha1.ResourceUsage{}
			err = c.Get(ctx, client.ObjectKeyFromObject(ru), got)
			if tc.wantDeleted {
				if !kerrors.IsNotFound(err) {
					t.Fatalf("ResourceUsage must be deleted, got %v", err)
				}
			} else if err != nil || !meta.FinalizerExists(got, v1alpha1.Finalizer) {
				t.Fatalf("ResourceUsage must retain finalizer, got %v, finalizers %v", err, got.Finalizers)
			}
			if !tc.missingTarget {
				gotTarget := &accountv1alpha1.ServiceBinding{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(target), gotTarget); err != nil || gotTarget.GetUID() != target.GetUID() {
					t.Fatalf("target must survive unchanged in identity, got %v, UID %s", err, gotTarget.GetUID())
				}
			}
		})
	}
}

func TestReplacementRetainsSourceProtection(t *testing.T) {
	for _, target := range []resource.Managed{&accountv1alpha1.ServiceBinding{}, &accountv1alpha1.Entitlement{}} {
		t.Run(fmt.Sprintf("%T", target), func(t *testing.T) {
			ctx := context.Background()
			scheme := runtime.NewScheme()
			if err := v1alpha1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			if err := accountv1alpha1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			source := &accountv1alpha1.Subaccount{ObjectMeta: metav1.ObjectMeta{Name: "source", UID: "source-uid"}}
			source.SetGroupVersionKind(accountv1alpha1.SubaccountGroupVersionKind)
			target.SetName("dependent")
			target.SetUID("new-target-uid")
			gvks, _, err := scheme.ObjectKinds(target)
			if err != nil {
				t.Fatal(err)
			}
			target.GetObjectKind().SetGroupVersionKind(gvks[0])
			oldUsage := targetIdentityUsage(target)
			now := metav1.Now()
			oldUsage.SetDeletionTimestamp(&now)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(source, target, oldUsage).Build()
			tracker := tracking.NewDefaultReferenceResolverTracker(c)
			if err := tracker.CreateTrackingReference(ctx, target, xpv1.Reference{Name: source.Name}, accountv1alpha1.SubaccountGroupVersionKind); err != nil {
				t.Fatal(err)
			}
			r := NewReconciler(&resourcefake.Manager{Client: c, Scheme: scheme})
			if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(oldUsage)}); err != nil {
				t.Fatal(err)
			}
			if err := c.Get(ctx, client.ObjectKeyFromObject(oldUsage), &v1alpha1.ResourceUsage{}); !kerrors.IsNotFound(err) {
				t.Fatalf("old ResourceUsage must be deleted, got %v", err)
			}
			usages := &v1alpha1.ResourceUsageList{}
			if err := c.List(ctx, usages); err != nil {
				t.Fatal(err)
			}
			if len(usages.Items) != 1 {
				t.Fatalf("replacement must retain its own ResourceUsage, got %d", len(usages.Items))
			}
			newUsage := &usages.Items[0]
			if newUsage.Name != "source-uid.new-target-uid" || newUsage.Labels[v1alpha1.LabelKeyTargetUid] != string(target.GetUID()) || newUsage.Spec.TargetReference.UID != target.GetUID() {
				t.Fatalf("replacement ResourceUsage has incorrect identity: %+v", newUsage)
			}
			owners := newUsage.GetOwnerReferences()
			if len(owners) != 1 || owners[0].UID != target.GetUID() {
				t.Fatalf("replacement must own its own ResourceUsage, got %v", owners)
			}
			tracker.SetConditions(ctx, source)
			if !tracker.DeleteShouldBeBlocked(source) {
				t.Fatal("source deletion must remain blocked by the replacement's ResourceUsage")
			}
			if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(newUsage)}); err != nil {
				t.Fatal(err)
			}
			if err := c.Get(ctx, client.ObjectKeyFromObject(newUsage), newUsage); err != nil || !meta.FinalizerExists(newUsage, v1alpha1.Finalizer) {
				t.Fatalf("replacement ResourceUsage must retain finalizer, got %v", err)
			}
		})
	}
}

func targetIdentityUsage(target resource.Managed) *v1alpha1.ResourceUsage {
	return &v1alpha1.ResourceUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "source-uid.old-target-uid",
			UID:        "usage-uid",
			Finalizers: []string{v1alpha1.Finalizer},
			Labels: map[string]string{
				v1alpha1.LabelKeySourceUid: "source-uid",
				v1alpha1.LabelKeyTargetUid: "old-target-uid",
			},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: target.GetObjectKind().GroupVersionKind().GroupVersion().String(), Kind: target.GetObjectKind().GroupVersionKind().Kind, Name: target.GetName(), UID: "old-target-uid"}},
		},
		Spec: v1alpha1.ResourceUsageSpec{
			SourceReference: xpv1.TypedReference{APIVersion: accountv1alpha1.SubaccountGroupVersionKind.GroupVersion().String(), Kind: "Subaccount", Name: "source"},
			TargetReference: xpv1.TypedReference{APIVersion: target.GetObjectKind().GroupVersionKind().GroupVersion().String(), Kind: target.GetObjectKind().GroupVersionKind().Kind, Name: target.GetName()},
		},
	}
}
