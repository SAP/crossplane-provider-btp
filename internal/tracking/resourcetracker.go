package tracking

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/reflectwalk"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
)

const (
	errCouldNotGetResourceUsage = "ResourceUsages could not be retrieved"
)

type DefaultReferenceResolverTracker struct {
	c client.Client
	a resource.Applicator
}

func NewDefaultReferenceResolverTracker(c client.Client) *DefaultReferenceResolverTracker {
	return &DefaultReferenceResolverTracker{
		c: c,
		a: resource.NewAPIUpdatingApplicator(c),
	}

}

// CreateTrackingReference creates a tracking reference for the given managed resource.
// It does not use the generic Track method because it has to be configured in a way Upjet does not support.
func (r *DefaultReferenceResolverTracker) CreateTrackingReference(
	ctx context.Context,
	cr resource.Managed,
	reference xpv1.Reference,
	gvk schema.GroupVersionKind,
) error {

	err := r.createTracking(ctx, cr, ResolvedReference{
		Reference:  reference,
		Group:      gvk.Group,
		Kind:       gvk.Kind,
		ApiVersion: gvk.Version,
	})

	if err != nil {
		return err
	}

	return nil
}

// Track finds all references in the given managed resource and creates tracking resources for them.
// It skips fields that do not have the `reference-group`, `reference-kind` and `reference-apiversion` tags.
func (r *DefaultReferenceResolverTracker) Track(ctx context.Context, mg resource.Managed) error {
	if hasIgnoreAnnotation(mg) {
		return nil
	}

	references, err := r.findReferences(mg)
	if err != nil {
		return err
	}

	for _, reference := range references {
		err := r.createTracking(ctx, mg, reference)
		if err != nil {
			return err
		}
	}
	return nil
}

func hasIgnoreAnnotation(mg resource.Managed) bool {
	_, ok := mg.GetAnnotations()[v1alpha1.AnnotationIgnoreReferences]
	return ok
}

// findReferences generically finds all references in the given resource.
// It uses reflection to walk through the resource and find all fields that are of type xpv1.Reference.
func (r *DefaultReferenceResolverTracker) findReferences(res interface{}) ([]ResolvedReference, error) {
	w := new(ReferenceWalker)
	w.Client = r.c
	w.Ctx = context.Background()
	err := reflectwalk.Walk(res, w)
	if err != nil {
		return nil, err
	}
	w.Fields = lo.FindUniques(w.Fields)
	return w.Fields, err
}

type ReferenceWalker struct {
	Fields []ResolvedReference
	Client client.Client
	Ctx    context.Context
}

type ResolvedReference struct {
	Reference  xpv1.Reference
	Group      string
	Kind       string
	ApiVersion string
}

func (w *ReferenceWalker) Struct(r reflect.Value) error {
	return nil
}
func (w *ReferenceWalker) StructField(sf reflect.StructField, v reflect.Value) error {
	if w.Fields == nil {
		w.Fields = make([]ResolvedReference, 0, 1)
	}
	desiredType := reflect.TypeOf(xpv1.Reference{})

	var value *xpv1.Reference

	if isTypePointer(sf, v, desiredType) {
		value = v.Interface().(*xpv1.Reference)
	}

	if sf.Type == desiredType {
		value = lo.ToPtr(v.Interface().(xpv1.Reference))
	}
	if value != nil {
		ref, done := w.createResolvedReference(sf, value)
		if !done {
			return nil
		}
		w.Fields = append(w.Fields, ref)
	}

	return nil
}
func isTypePointer(sf reflect.StructField, v reflect.Value, desiredType reflect.Type) bool {
	if sf.Type.Kind() != reflect.Pointer || v.IsZero() {
		return false
	}
	indirectType := reflect.Indirect(v)
	return indirectType.Type() == desiredType

}

func (w *ReferenceWalker) createResolvedReference(sf reflect.StructField, value *xpv1.Reference) (
	ResolvedReference,
	bool,
) {
	group, ok := sf.Tag.Lookup("reference-group")
	if !ok {
		return ResolvedReference{}, ok
	}
	kind, ok := sf.Tag.Lookup("reference-kind")
	if !ok {
		return ResolvedReference{}, ok
	}
	apiversion, ok := sf.Tag.Lookup("reference-apiversion")
	if !ok {
		return ResolvedReference{}, ok
	}
	ref := ResolvedReference{
		Reference:  *value.DeepCopy(),
		Group:      group,
		Kind:       kind,
		ApiVersion: apiversion,
	}
	return ref, true
}

func (r *DefaultReferenceResolverTracker) createTracking(
	ctx context.Context,
	target resource.Managed,
	source ResolvedReference,
) error {
	ru := v1alpha1.ResourceUsage{}
	targetGvk := target.GetObjectKind().GroupVersionKind()

	sourceRes, err := r.getResource(
		ctx,
		source.Group,
		source.ApiVersion,
		source.Kind,
		source.Reference.Name,
	)
	if err != nil {
		return err
	}

	ownerRef := meta.AsOwner(
		meta.TypedReferenceTo(
			target,
			targetGvk,
		),
	)
	ownerRef.BlockOwnerDeletion = lo.ToPtr(false)
	ru.SetName(fmt.Sprintf("%s.%s", sourceRes.GetUID(), target.GetUID()))
	ru.SetOwnerReferences(
		[]metav1.OwnerReference{
			ownerRef,
		},
	)
	ru.SetLabels(
		map[string]string{
			v1alpha1.LabelKeyTargetUid: string(target.GetUID()),
			v1alpha1.LabelKeySourceUid: string(sourceRes.GetUID()),
		},
	)

	ru.Spec.SourceReference = *meta.TypedReferenceTo(sourceRes, sourceRes.GroupVersionKind())
	ru.Spec.TargetReference = *meta.TypedReferenceTo(target, targetGvk)
	err = r.a.Apply(
		ctx, &ru,
		resource.MustBeControllableBy(target.GetUID()),
		resource.AllowUpdateIf(specsDiffer),
	)
	return resource.Ignore(resource.IsNotAllowed, err)
}

func specsDiffer(current runtime.Object, desired runtime.Object) bool {
	c := current.(*v1alpha1.ResourceUsage)
	d := desired.(*v1alpha1.ResourceUsage)
	return !cmp.Equal(c.GetLabels(), d.GetLabels()) || !cmp.Equal(c.Spec, d.Spec)
}

func (r *DefaultReferenceResolverTracker) getResource(
	ctx context.Context, group string, version string, kind string, name string,
) (
	*metav1.PartialObjectMetadata, error,
) {

	resourceId := schema.GroupVersionKind{
		Group:   strings.ToLower(group),
		Version: strings.ToLower(version),
		Kind:    strings.ToLower(kind),
	}
	return r.getResourceByGVK(ctx, resourceId, name)
}

func (r *DefaultReferenceResolverTracker) getResourceByGVK(
	ctx context.Context, gvr schema.GroupVersionKind, name string,
) (
	*metav1.PartialObjectMetadata, error,
) {
	object := metav1.PartialObjectMetadata{}
	object.SetGroupVersionKind(gvr)
	object.SetName(name)

	err := r.c.Get(ctx, client.ObjectKeyFromObject(&object), &object)

	if err != nil {
		return nil, err
	}

	return &object, nil
}

func (r *DefaultReferenceResolverTracker) hasUsages(
	ctx context.Context,
	managed resource.Managed,
) (bool, error) {
	usages, err := r.getUsagesBySource(ctx, managed)
	return len(usages.Items) > 0, err
}

func (r *DefaultReferenceResolverTracker) getUsagesBySource(
	ctx context.Context,
	managed resource.Managed,
) (v1alpha1.ResourceUsageList, error) {
	l := v1alpha1.ResourceUsageList{}
	uid := managed.GetUID()
	err := r.c.List(ctx, &l, client.MatchingLabels{v1alpha1.LabelKeySourceUid: string(uid)})
	return l, errors.Wrap(err, errCouldNotGetResourceUsage)
}

func (r *DefaultReferenceResolverTracker) SetConditions(ctx context.Context, mg resource.Managed) {
	hasUsages, err := r.hasUsages(ctx, mg)
	mg.SetConditions(v1alpha1.NewInUseCondition(hasUsages, err))
}

func (r *DefaultReferenceResolverTracker) ResolveSource(
	ctx context.Context,
	ru v1alpha1.ResourceUsage,
) (*metav1.PartialObjectMetadata, error) {
	return r.getResourceByGVK(ctx, ru.Spec.SourceReference.GroupVersionKind(), ru.Spec.SourceReference.Name)
}

func (r *DefaultReferenceResolverTracker) ResolveTarget(
	ctx context.Context,
	ru v1alpha1.ResourceUsage,
) (*metav1.PartialObjectMetadata, error) {
	return r.getResourceByGVK(ctx, ru.Spec.TargetReference.GroupVersionKind(), ru.Spec.TargetReference.Name)
}

func (r *DefaultReferenceResolverTracker) DeleteShouldBeBlocked(mg resource.Managed) bool {
	if hasIgnoreAnnotation(mg) {
		return false
	}
	return mg.GetCondition(v1alpha1.UseCondition).Reason == v1alpha1.InUseReason
}

type ReferenceResolverTracker interface {
	Track(ctx context.Context, mg resource.Managed) error
	SetConditions(ctx context.Context, mg resource.Managed)
	ResolveSource(
		ctx context.Context,
		ru v1alpha1.ResourceUsage,
	) (*metav1.PartialObjectMetadata, error)
	ResolveTarget(
		ctx context.Context,
		ru v1alpha1.ResourceUsage,
	) (*metav1.PartialObjectMetadata, error)
	DeleteShouldBeBlocked(mg resource.Managed) bool
}
