package adoption

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

// managedResource builds a fresh managed resource per case, so no case can leak
// annotations into another. An empty name defaults to "cr".
func managedResource(name string, annotations map[string]string, externalName string, deleting bool) *fake.Managed {
	if name == "" {
		name = "cr"
	}
	mg := &fake.Managed{ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations}}
	if externalName != "" {
		meta.SetExternalName(mg, externalName)
	}
	if deleting {
		now := metav1.NewTime(time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC))
		mg.SetDeletionTimestamp(&now)
	}
	return mg
}

func TestPending(t *testing.T) {
	type args struct {
		name         string
		annotations  map[string]string
		externalName string
		deleting     bool
	}
	type want struct {
		pending bool
		err     error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NoAnnotation": {
			reason: "A resource that did not opt in must never trigger a lookup.",
			args:   args{},
			want:   want{pending: false},
		},
		"OptedInWithoutExternalName": {
			reason: "An opted-in resource with no identity yet is the one case a lookup runs.",
			args:   args{annotations: map[string]string{Annotation: "true"}},
			want:   want{pending: true},
		},
		"ExplicitFalse": {
			reason: "\"false\" is a valid way to switch the lookup off without removing the annotation.",
			args:   args{annotations: map[string]string{Annotation: "false"}},
			want:   want{pending: false},
		},
		"ExternalNameAlreadySet": {
			reason: "An external-name, user-set or adopted earlier, always wins; the lookup must not re-run or overwrite it.",
			args:   args{annotations: map[string]string{Annotation: "true"}, externalName: "80540c06-2955-4bce-9c43-ad78fecc7f62"},
			want:   want{pending: false},
		},
		"ExternalNameIsObjectName": {
			reason: "Crossplane's default initializer and older provider versions write metadata.name into the external-name; that is not a BTP identifier, so the lookup still runs.",
			args:   args{name: "my-instance", annotations: map[string]string{Annotation: "true"}, externalName: "my-instance"},
			want:   want{pending: true},
		},
		"ExternalNameIsGUIDObjectName": {
			reason: "An object deliberately named after its BTP GUID is taken at its word: that external-name is an identifier, not a default.",
			args:   args{name: "80540c06-2955-4bce-9c43-ad78fecc7f62", annotations: map[string]string{Annotation: "true"}, externalName: "80540c06-2955-4bce-9c43-ad78fecc7f62"},
			want:   want{pending: false},
		},
		"ExternalNameIsUpperCaseGUIDObjectName": {
			reason: "BTP reports some IDs (Kyma environments) in upper case; they are GUIDs all the same.",
			args:   args{name: "29D6957E-B781-440D-8FAA-CD3589294969", annotations: map[string]string{Annotation: "true"}, externalName: "29D6957E-B781-440D-8FAA-CD3589294969"},
			want:   want{pending: false},
		},
		"ExternalNameIsCompoundKey": {
			reason: "Compound identifiers (instanceID/bindingID, appName/planName) are set identifiers, never defaults.",
			args:   args{name: "my-sm", annotations: map[string]string{Annotation: "true"}, externalName: "si-1/sb-1"},
			want:   want{pending: false},
		},
		"Deleting": {
			reason: "Adopting during deletion would make the provider delete a resource it never managed.",
			args:   args{annotations: map[string]string{Annotation: "true"}, deleting: true},
			want:   want{pending: false},
		},
		"InvalidValue": {
			reason: "A typo such as \"yes\" must surface as an error rather than silently creating a duplicate.",
			args:   args{annotations: map[string]string{Annotation: "yes"}},
			want:   want{err: errInvalidValue("yes")},
		},
		"InvalidValueWithExternalNameSet": {
			reason: "The value is validated even when the lookup would be skipped, so typos never hide.",
			args:   args{annotations: map[string]string{Annotation: "True"}, externalName: "80540c06-2955-4bce-9c43-ad78fecc7f62"},
			want:   want{err: errInvalidValue("True")},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mg := managedResource(tc.args.name, tc.args.annotations, tc.args.externalName, tc.args.deleting)

			got, err := Pending(mg)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("%s\nPending(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.pending, got); diff != "" {
				t.Errorf("%s\nPending(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// recorder captures emitted events.
type recorder struct {
	events []event.Event
}

func (r *recorder) Event(_ runtime.Object, e event.Event) {
	r.events = append(r.events, e)
}

func (r *recorder) WithAnnotations(...string) event.Recorder {
	return r
}

func TestCommit(t *testing.T) {
	errBoom := errors.New("boom")

	type args struct {
		kube client.Writer
		rec  *recorder
		id   string
		key  string
	}
	type want struct {
		externalName string
		events       []event.Event
		err          error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"PersistsAndRequeues": {
			reason: "A committed adoption must persist the external-name and requeue so Connect rebuilds clients against it.",
			args: args{
				kube: &test.MockClient{MockPatch: test.NewMockPatchFn(nil)},
				rec:  &recorder{},
				id:   "si-1",
				key:  "name=my-instance",
			},
			want: want{
				externalName: "si-1",
				events: []event.Event{event.Normal(EventReasonAdopted,
					"Adopted existing BTP resource si-1 found by lookup (name=my-instance)")},
				err: ErrRequeueAfterAdoption,
			},
		},
		"UpdateFails": {
			reason: "A failed write must be returned and must not emit an adoption event that did not happen.",
			args: args{
				kube: &test.MockClient{MockPatch: test.NewMockPatchFn(errBoom)},
				rec:  &recorder{},
				id:   "si-1",
				key:  "name=my-instance",
			},
			want: want{
				externalName: "si-1",
				err:          errors.Wrap(errBoom, errPersist),
			},
		},
		"NilRecorder": {
			reason: "Controllers wired without a recorder (unit tests) must still adopt.",
			args: args{
				kube: &test.MockClient{MockPatch: test.NewMockPatchFn(nil)},
				id:   "si-1",
				key:  "name=my-instance",
			},
			want: want{
				externalName: "si-1",
				err:          ErrRequeueAfterAdoption,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mg := managedResource("", map[string]string{Annotation: "true"}, "", false)

			var rec event.Recorder
			if tc.args.rec != nil {
				rec = tc.args.rec
			}
			err := Commit(context.Background(), tc.args.kube, rec, mg, tc.args.id, tc.args.key)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("%s\nCommit(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.externalName, meta.GetExternalName(mg)); diff != "" {
				t.Errorf("%s\nCommit(...): -want external-name, +got external-name:\n%s", tc.reason, diff)
			}
			var got []event.Event
			if tc.args.rec != nil {
				got = tc.args.rec.events
			}
			if diff := cmp.Diff(tc.want.events, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("%s\nCommit(...): -want events, +got events:\n%s", tc.reason, diff)
			}
		})
	}
}

// TestCommitPatchesOnlyTheExternalName pins Commit's ownership: it writes the
// external-name and the managed finalizer, whatever else the reconcile changed
// in memory stays unwritten, and the write is guarded by the resourceVersion
// the object was read at.
//
// It uses a real API type: fake.Managed inlines ObjectMeta, so it would not
// serialize the patch the way the API server receives it.
func TestCommitPatchesOnlyTheExternalName(t *testing.T) {
	mg := &v1alpha1.ServiceInstance{}
	mg.SetName("cr")
	mg.SetAnnotations(map[string]string{Annotation: "true"})
	mg.SetResourceVersion("42")
	mg.SetLabels(map[string]string{"changed-in-memory": "earlier-in-the-reconcile"})

	var got string
	kube := &test.MockClient{MockPatch: func(_ context.Context, obj client.Object, patch client.Patch, _ ...client.PatchOption) error {
		data, err := patch.Data(obj)
		if err != nil {
			return err
		}
		got = string(data)
		return nil
	}}

	if err := Commit(context.Background(), kube, nil, mg, "si-1", "name=my-instance"); !errors.Is(err, ErrRequeueAfterAdoption) {
		t.Fatalf("Commit(...): want ErrRequeueAfterAdoption, got %v", err)
	}

	want := `{"metadata":{"annotations":{"crossplane.io/external-name":"si-1"},"finalizers":["finalizer.managedresource.crossplane.io"],"resourceVersion":"42"}}`
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Commit(...): -want patch, +got patch:\n%s", diff)
	}
}

// TestCommitGuardsDeletion pins why Commit adds the managed finalizer: the
// adopting reconcile ends before the managed reconciler adds it, and a CR
// deleted in that window would vanish without the provider deleting the BTP
// resource it just took over.
func TestCommitGuardsDeletion(t *testing.T) {
	cases := map[string]struct {
		reason     string
		finalizers []string
		want       []string
	}{
		"AddsTheFinalizer": {
			reason: "A freshly adopted resource gets the finalizer in the same write as its identity.",
			want:   []string{managed.FinalizerName},
		},
		"KeepsAnExistingFinalizerOnce": {
			reason:     "A finalizer that is already there is neither duplicated nor reordered.",
			finalizers: []string{"other.example.org", managed.FinalizerName},
			want:       []string{"other.example.org", managed.FinalizerName},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mg := &v1alpha1.ServiceInstance{}
			mg.SetName("cr")
			mg.SetFinalizers(tc.finalizers)

			err := Commit(context.Background(), &test.MockClient{MockPatch: test.NewMockPatchFn(nil)}, nil, mg, "si-1", "name=my-instance")

			if !errors.Is(err, ErrRequeueAfterAdoption) {
				t.Fatalf("%s\nCommit(...): want ErrRequeueAfterAdoption, got %v", tc.reason, err)
			}
			if diff := cmp.Diff(tc.want, mg.GetFinalizers()); diff != "" {
				t.Errorf("%s\nCommit(...): -want finalizers, +got finalizers:\n%s", tc.reason, diff)
			}
		})
	}
}
