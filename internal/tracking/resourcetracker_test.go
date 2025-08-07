package tracking

import (
	"fmt"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/samber/lo"
	"golang.org/x/net/context"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
)

type StructWithoutReference struct {
	Foo string
}

type StructWithReference struct {
	Foo string
	Ref xpv1.Reference `reference-group:"group" reference-kind:"kind" reference-apiversion:"v1"`
}

func NewStructWithReference() *StructWithReference {
	return &StructWithReference{Ref: xpv1.Reference{}}
}

type StructWithReferencePtr struct {
	Foo string
	Ref *xpv1.Reference `reference-group:"group" reference-kind:"kind" reference-apiversion:"v1"`
}

func NewStructWithReferencePtr() *StructWithReferencePtr {
	return &StructWithReferencePtr{Ref: lo.ToPtr(xpv1.Reference{})}
}

type StructWithoutTags struct {
	Foo string
	Ref *xpv1.Reference
}

func NewStructWithoutTags() *StructWithoutTags {
	return &StructWithoutTags{
		Foo: "",
		Ref: lo.ToPtr(xpv1.Reference{}),
	}
}

type StructPartialTags struct {
	Ref1 *xpv1.Reference `reference-group:"group1" reference-kind:"kind" `
	Ref2 *xpv1.Reference `reference-group:"group2" reference-kind:"kind" `
	Ref3 *xpv1.Reference `reference-group:"group3"`
	Ref4 *xpv1.Reference `reference-apiversion:"v4"`
	Ref5 *xpv1.Reference `reference-group:"group5" reference-kind:"kind" reference-apiversion:"v1"`
}

func NewStructPartialTags() *StructPartialTags {
	return &StructPartialTags{
		Ref1: lo.ToPtr(xpv1.Reference{}),
		Ref2: lo.ToPtr(xpv1.Reference{}),
		Ref3: lo.ToPtr(xpv1.Reference{}),
		Ref4: lo.ToPtr(xpv1.Reference{}),
		Ref5: lo.ToPtr(xpv1.Reference{}),
	}
}

func Test_findReferences(t *testing.T) {
	tracker := NewDefaultReferenceResolverTracker(nil)

	type args struct {
		res interface{}
	}
	tests := []struct {
		name string
		args args
		want []ResolvedReference
	}{
		{
			name: "With ManagedResource",
			args: args{
				res: &v1alpha1.ServiceManager{
					Spec: v1alpha1.ServiceManagerSpec{
						ResourceSpec: xpv1.ResourceSpec{},
						ForProvider: v1alpha1.ServiceManagerParameters{
							SubaccountRef: &xpv1.Reference{
								Name: "asd",
							},
						},
					},
				},
			},
			want: []ResolvedReference{
				{
					Reference: xpv1.Reference{
						Name: "asd",
					},
					Group:      "account.btp.sap.crossplane.io",
					Kind:       "Subaccount",
					ApiVersion: "v1alpha1",
				},
			},
		},
		{
			name: "Without Tags",
			args: args{
				res: NewStructWithoutTags(),
			},
			want: []ResolvedReference{},
		},
		{
			name: "Without Reference",
			args: args{
				res: new(StructWithoutReference),
			},
			want: []ResolvedReference{},
		},
		{
			name: "Reference as Ptr",
			args: args{
				res: NewStructWithReferencePtr(),
			},
			want: []ResolvedReference{
				{
					Reference:  xpv1.Reference{},
					Group:      "group",
					Kind:       "kind",
					ApiVersion: "v1",
				},
			},
		},
		{
			name: "Reference with Tags",
			args: args{
				res: NewStructWithReference(),
			},
			want: []ResolvedReference{
				{
					Reference:  xpv1.Reference{},
					Group:      "group",
					Kind:       "kind",
					ApiVersion: "v1",
				},
			},
		},
		{
			name: "With Partically created tags",
			args: args{
				res: NewStructPartialTags(),
			},
			want: []ResolvedReference{
				{
					Reference:  xpv1.Reference{},
					Group:      "group5",
					Kind:       "kind",
					ApiVersion: "v1",
				},
			},
		},
		{
			name: "Tags but no reference set",
			args: args{
				res: StructWithReferencePtr{Ref: nil},
			},
			want: []ResolvedReference{},
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				got, err := tracker.findReferences(tt.args.res)
				if err != nil {
					t.Errorf("\n%s\ne.findReferences(...): got:\n%s\n", tt.name, err)
				}
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("\n%s\ne.findReferences(...): -want, +got:\n%s\n", tt.name, diff)
				}
			},
		)
	}
}

func Test_Track(t *testing.T) {

	type args struct {
		mg                resource.Managed
		additionalObjects []kclient.Object
	}
	tests := []struct {
		name     string
		args     args
		want     []*providerv1alpha1.ResourceUsage
		err      error
		postfunc func(ctx context.Context, client kclient.WithWatch, testname string) error
	}{
		{
			name: "When there is an error to resolve a reference then an error is thrown",
			args: args{
				mg:                newFakeSubaccount(),
				additionalObjects: []kclient.Object{},
			},
			want: []*providerv1alpha1.ResourceUsage{},
			err: kerrors.NewNotFound(
				schema.GroupResource{
					Group:    "account.btp.sap.crossplane.io",
					Resource: "directories",
				},
				"fake-directory",
			),
		},
		{
			name: "When there is one reference then a resource usage object is created",
			args: args{
				mg: newFakeSubaccount(),
				additionalObjects: []kclient.Object{
					newFakeDirectory(),
				},
			},
			want: []*providerv1alpha1.ResourceUsage{
				newResourceUsage(newFakeDirectory(), newFakeSubaccount()),
			},
		},
		{
			name:     "When there is no reference then no resource usage object is created",
			postfunc: checkNoResourceUsagesExist(t),
			want:     []*providerv1alpha1.ResourceUsage{},
			args: args{
				mg:                newFakeDirectory(),
				additionalObjects: []kclient.Object{},
			},
		},

		{
			name:     "When there is one reference and ignore annotation then no resource usage object is created",
			postfunc: checkNoResourceUsagesExist(t),
			args: args{
				mg: addIgnoreAnnotation(newFakeSubaccount()),
				additionalObjects: []kclient.Object{
					newFakeDirectory(),
				},
			},
			want: []*providerv1alpha1.ResourceUsage{},
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				ctx := context.TODO()
				client, tracker := buildFakeClient(append(tt.args.additionalObjects, tt.args.mg), t)

				err := tracker.Track(ctx, tt.args.mg)

				if diff := cmp.Diff(tt.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\ne.Track(...): -want error, +got error:\n%s\n", tt.name, diff)
				}

				if tt.postfunc != nil {
					if err = tt.postfunc(ctx, client, tt.name); err != nil {
						t.Errorf("\n%s\ne.Track(...): got:\n%s\n", tt.name, err)
					}
				}

				for _, want := range tt.want {
					got := &providerv1alpha1.ResourceUsage{}
					if err = client.Get(ctx, kclient.ObjectKeyFromObject(want), got); err != nil {
						t.Errorf("\n%s\ne.Track(...): got:\n%s\n", tt.name, err)
					}
				}

			},
		)
	}

}

func checkNoResourceUsagesExist(t *testing.T) func(
	ctx context.Context,
	client kclient.WithWatch,
	testname string,
) error {
	return func(ctx context.Context, client kclient.WithWatch, testname string) error {
		list := &providerv1alpha1.ResourceUsageList{}
		err := client.List(ctx, list)
		if err != nil {
			return err
		}
		if diff := cmp.Diff(list.Items, []providerv1alpha1.ResourceUsage{}); diff != "" {
			t.Errorf("\n%s\ne.Track(...): -want, +got:\n%s\n", testname, diff)
		}
		return nil
	}
}
func buildFakeClient(initialObjects []kclient.Object, t *testing.T) (kclient.WithWatch, *DefaultReferenceResolverTracker) {
	builder := fake.NewClientBuilder()
	for _, object := range initialObjects {
		builder.WithObjects(object)
	}
	scheme := runtime.NewScheme()
	err := apis.AddToScheme(scheme)
	if err != nil {
		t.Fatal(err)
	}
	builder.WithScheme(scheme)
	client := builder.Build()
	tracker := NewDefaultReferenceResolverTracker(client)
	return client, tracker
}

func newFakeSubaccount() *v1alpha1.Subaccount {
	return &v1alpha1.Subaccount{
		ObjectMeta: metav1.ObjectMeta{Name: "fake-subaccount", UID: "subaccount-uid"},
		Spec: v1alpha1.SubaccountSpec{
			ForProvider: v1alpha1.SubaccountParameters{
				DirectoryRef: &xpv1.Reference{
					Name: "fake-directory",
				},
			},
		},
	}
}

func addIgnoreAnnotation(mg resource.Managed) resource.Managed {
	annotations := mg.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[providerv1alpha1.AnnotationIgnoreReferences] = "true"
	mg.SetAnnotations(annotations)
	return mg
}

func newFakeDirectory() *v1alpha1.Directory {
	return &v1alpha1.Directory{
		ObjectMeta: metav1.ObjectMeta{Name: "fake-directory", UID: "directory-uid"},
	}
}

func newResourceUsage(source resource.Managed, target resource.Managed) *providerv1alpha1.ResourceUsage {
	return &providerv1alpha1.ResourceUsage{
		TypeMeta: metav1.TypeMeta{
			Kind:       providerv1alpha1.ResourceUsageKind,
			APIVersion: "account.btp.sap.crossplane.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("%s.%s", source.GetUID(), target.GetUID()),
			ResourceVersion: "1",
			Labels: map[string]string{
				providerv1alpha1.LabelKeySourceUid: string(source.GetUID()),
				providerv1alpha1.LabelKeyTargetUid: string(target.GetUID()),
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:               target.GetName(),
					UID:                target.GetUID(),
					BlockOwnerDeletion: lo.ToPtr(false),
				},
			},
		},
		Spec: providerv1alpha1.ResourceUsageSpec{
			SourceReference: xpv1.TypedReference{
				APIVersion: "account.btp.sap.crossplane.io/v1alpha1",
				Kind:       "directories",
				Name:       source.GetName(),
				UID:        source.GetUID(),
			},
			TargetReference: xpv1.TypedReference{
				Name: target.GetName(),
				UID:  target.GetUID(),
			},
		},
	}
}

func Test_HasUsages(t *testing.T) {
	type args struct {
		mg                resource.Managed
		additionalObjects []kclient.Object
	}
	tests := []struct {
		name string
		err  error
		want bool
		args args
	}{
		{
			name: "When there are no usages for a resource return an false",
			args: args{
				mg:                newFakeSubaccount(),
				additionalObjects: []kclient.Object{},
			},
			want: false,
		},
		{
			name: "When there are usages for a resource found then true",
			args: args{
				mg: newFakeDirectory(),
				additionalObjects: []kclient.Object{
					newResourceUsage(newFakeDirectory(), newFakeSubaccount()),
					newFakeSubaccount(),
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				ctx := context.TODO()
				_, tracker := buildFakeClient(append(tt.args.additionalObjects, tt.args.mg), t)

				got, err := tracker.hasUsages(ctx, tt.args.mg)

				if diff := cmp.Diff(tt.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\ne.hasUsages(...): -want error, +got error:\n%s\n", tt.name, diff)
				}

				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("\n%s\ne.hasUsages(...): -want, +got:\n%s\n", tt.name, diff)
				}
			},
		)
	}
}

func Test_SetConditions(t *testing.T) {
	type args struct {
		mg                resource.Managed
		additionalObjects []kclient.Object
	}
	tests := []struct {
		name string
		err  error
		want xpv1.Condition
		args args
	}{
		{
			name: "When there are no usages then a not in use condition will be set",
			args: args{
				mg:                newFakeDirectory(),
				additionalObjects: []kclient.Object{},
			},
			want: providerv1alpha1.NotInUse(),
		},
		{
			name: "When there are usages for a resource a in use condition will be set",
			args: args{
				mg: newFakeDirectory(),
				additionalObjects: []kclient.Object{
					newResourceUsage(newFakeDirectory(), newFakeSubaccount()),
					newFakeSubaccount(),
				},
			},
			want: providerv1alpha1.InUse(),
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				ctx := context.TODO()
				_, tracker := buildFakeClient(append(tt.args.additionalObjects, tt.args.mg), t)

				tracker.SetConditions(ctx, tt.args.mg)

				got := tt.args.mg.GetCondition(providerv1alpha1.UseCondition)
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("\n%s\ne.SetConditions(...): -want, +got:\n%s\n", tt.name, diff)
				}
			},
		)
	}
}

func TestDeleteShouldBeBlocked(t *testing.T) {
	type args struct {
		mg                resource.Managed
		additionalObjects []kclient.Object
	}
	tests := []struct {
		name string
		err  error
		want bool
		args args
	}{
		{
			name: "When there are is a in Use condition with InUse Reason then delete should be blocked",
			args: args{
				mg:                addCondition(newFakeDirectory(), providerv1alpha1.InUse()),
				additionalObjects: []kclient.Object{},
			},
			want: true,
		},
		{
			name: "When there are is a in Use condition with NotInuse Reason then delete should not be blocked",
			args: args{
				mg: addCondition(newFakeDirectory(), providerv1alpha1.NotInUse()),
			},
			want: false,
		},
		{
			name: "When there are is no use condition then delete should not be blocked",
			args: args{
				mg: newFakeDirectory(),
			},
			want: false,
		},
		{
			name: "When there are is a in Use condition with InUse Reason and ignore annotation then delete should not be blocked",
			args: args{
				mg:                addIgnoreAnnotation(addCondition(newFakeDirectory(), providerv1alpha1.InUse())),
				additionalObjects: []kclient.Object{},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				_, tracker := buildFakeClient(append(tt.args.additionalObjects, tt.args.mg), t)

				got := tracker.DeleteShouldBeBlocked(tt.args.mg)

				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("\n%s\ne.DeleteShouldBeBlocked(...): -want, +got:\n%s\n", tt.name, diff)
				}
			},
		)
	}
}

func addCondition(mg resource.Managed, condition xpv1.Condition) resource.Managed {
	mg.SetConditions(condition)
	return mg
}

func TestResolveTarget(t *testing.T) {
	type args struct {
		ru                providerv1alpha1.ResourceUsage
		additionalObjects []kclient.Object
	}
	tests := []struct {
		name string
		err  error
		want *metav1.PartialObjectMetadata
		args args
	}{
		{
			name: "Resolves existing target",
			args: args{
				ru: *newReconciledResourceUsage(newFakeSubaccount(), newFakeDirectory()),
				additionalObjects: []kclient.Object{
					newFakeSubaccount(),
					newFakeDirectory(),
				},
			},
			want: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       "directories",
					APIVersion: "account.btp.sap.crossplane.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "fake-directory",
					UID:             "directory-uid",
					ResourceVersion: "999",
				},
			},
		},
		{
			name: "Target cannot be resolved",
			args: args{
				ru: *newReconciledResourceUsage(newFakeSubaccount(), newFakeDirectory()),
				additionalObjects: []kclient.Object{
					newFakeSubaccount(),
				},
			},
			err: kerrors.NewNotFound(
				schema.GroupResource{
					Group:    "account.btp.sap.crossplane.io",
					Resource: "directories",
				},
				"fake-directory",
			),
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				_, tracker := buildFakeClient(append(tt.args.additionalObjects, &tt.args.ru), t)

				_, err := tracker.ResolveTarget(context.TODO(), tt.args.ru)

				if diff := cmp.Diff(tt.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\ne.ResolveTarget(...): -want error, +got error:\n%s\n", tt.name, diff)
				}
			},
		)
	}
}

func newReconciledResourceUsage(source resource.Managed, target resource.Managed) *providerv1alpha1.ResourceUsage {
	return &providerv1alpha1.ResourceUsage{
		TypeMeta: metav1.TypeMeta{
			Kind:       providerv1alpha1.ResourceUsageKind,
			APIVersion: "account.btp.sap.crossplane.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("%s.%s", source.GetUID(), target.GetUID()),
			ResourceVersion: "1",
			Labels: map[string]string{
				providerv1alpha1.LabelKeySourceUid: string(source.GetUID()),
				providerv1alpha1.LabelKeyTargetUid: string(target.GetUID()),
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:               target.GetName(),
					UID:                target.GetUID(),
					BlockOwnerDeletion: lo.ToPtr(false),
				},
			},
		},
		Spec: providerv1alpha1.ResourceUsageSpec{
			SourceReference: xpv1.TypedReference{
				APIVersion: "account.btp.sap.crossplane.io/v1alpha1",
				Kind:       "subaccount",
				Name:       source.GetName(),
				UID:        source.GetUID(),
			},
			TargetReference: xpv1.TypedReference{
				Name:       target.GetName(),
				UID:        target.GetUID(),
				APIVersion: "account.btp.sap.crossplane.io/v1alpha1",
				Kind:       "directory",
			},
		},
	}
}

func TestResolveSource(t *testing.T) {
	type args struct {
		ru                providerv1alpha1.ResourceUsage
		additionalObjects []kclient.Object
	}
	tests := []struct {
		name string
		err  error
		want *metav1.PartialObjectMetadata
		args args
	}{
		{
			name: "Resolves existing source",
			args: args{
				ru: *newReconciledResourceUsage(newFakeSubaccount(), newFakeDirectory()),
				additionalObjects: []kclient.Object{
					newFakeSubaccount(),
					newFakeDirectory(),
				},
			},
			want: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       "subaccount",
					APIVersion: "account.btp.sap.crossplane.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "fake-subaccount",
					UID:             "subaccount-uid",
					ResourceVersion: "999",
				},
			},
		},
		{
			name: "Source cannot be resolved",
			args: args{
				ru: *newReconciledResourceUsage(newFakeSubaccount(), newFakeDirectory()),
				additionalObjects: []kclient.Object{
					newFakeDirectory(),
				},
			},
			err: kerrors.NewNotFound(
				schema.GroupResource{
					Group:    "account.btp.sap.crossplane.io",
					Resource: "subaccounts",
				},
				"fake-subaccount",
			),
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				_, tracker := buildFakeClient(append(tt.args.additionalObjects, &tt.args.ru), t)

				_, err := tracker.ResolveSource(context.TODO(), tt.args.ru)

				if diff := cmp.Diff(tt.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\ne.ResolveSource(...): -want error, +got error:\n%s\n", tt.name, diff)
				}
			},
		)
	}
}
