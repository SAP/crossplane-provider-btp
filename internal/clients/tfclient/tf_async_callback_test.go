package tfclient

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	runtimeobj "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	ujresource "github.com/crossplane/upjet/v2/pkg/resource"
	tferrors "github.com/crossplane/upjet/v2/pkg/terraform/errors"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

// callbackFor returns the terraform callback for the given verb, so the three
// verbs can be driven from one table.
func callbackFor(t *testing.T, ac *APICallbacks, verb string, name types.NamespacedName) func(error, context.Context) error {
	t.Helper()
	switch verb {
	case "create":
		return ac.Create(name, true)
	case "update":
		return ac.Update(name, true)
	case "destroy":
		return ac.Destroy(name, true)
	}
	t.Fatalf("unknown verb %q", verb)
	return nil
}

// saveSpy records what the SaveConditionsFn was handed.
type saveSpy struct {
	calls      int
	gotName    types.NamespacedName
	gotConds   []xpv1.Condition
	returnErr  error
	gotKubeNil bool
}

func (s *saveSpy) fn() SaveConditionsFn {
	return func(_ context.Context, kube client.Client, name types.NamespacedName, conditions ...xpv1.Condition) error {
		s.calls++
		s.gotName = name
		s.gotConds = conditions
		s.gotKubeNil = kube == nil
		return s.returnErr
	}
}

func (s *saveSpy) conditionTypes() []string {
	out := make([]string, 0, len(s.gotConds))
	for _, c := range s.gotConds {
		out = append(out, string(c.Type))
	}
	return out
}

// logSink captures whether a message was logged at error or info level.
type logSink struct {
	errors []string
	infos  []string
}

func (l *logSink) Init(logr.RuntimeInfo)               {}
func (l *logSink) Enabled(int) bool                    { return true }
func (l *logSink) WithValues(...any) logr.LogSink      { return l }
func (l *logSink) WithName(string) logr.LogSink        { return l }
func (l *logSink) Info(_ int, msg string, _ ...any)    { l.infos = append(l.infos, msg) }
func (l *logSink) Error(_ error, msg string, _ ...any) { l.errors = append(l.errors, msg) }

// eventSpy collects emitted events.
type eventSpy struct {
	events []event.Event
	objs   []runtimeobj.Object
}

func (e *eventSpy) Event(obj runtimeobj.Object, ev event.Event) {
	e.objs = append(e.objs, obj)
	e.events = append(e.events, ev)
}
func (e *eventSpy) WithAnnotations(...string) event.Recorder { return e }

func TestAPICallbacks_ResolvesShadowName(t *testing.T) {
	// A ServiceInstance named "x" is driven through a terraform shadow named
	// "TF-x" with no namespace. Upjet hands the callbacks the shadow identity;
	// the result must land on the ServiceInstance.
	for _, verb := range []string{"create", "update", "destroy"} {
		t.Run(verb, func(t *testing.T) {
			spy := &saveSpy{}
			ac := NewAPICallbacks(&test.MockClient{}, spy.fn())

			cb := callbackFor(t, ac, verb, types.NamespacedName{Name: "TF-x"})
			if err := cb(nil, context.Background()); err != nil {
				t.Fatalf("callback returned unexpected error: %v", err)
			}

			if spy.calls != 1 {
				t.Fatalf("expected exactly one save call, got %d", spy.calls)
			}
			want := types.NamespacedName{Name: "x"}
			if diff := cmp.Diff(want, spy.gotName); diff != "" {
				t.Errorf("resolved name mismatch (-want, +got):\n%s", diff)
			}
			wantConds := []string{ujresource.TypeLastAsyncOperation, ujresource.TypeAsyncOperation}
			if diff := cmp.Diff(wantConds, spy.conditionTypes()); diff != "" {
				t.Errorf("persisted condition types mismatch (-want, +got):\n%s", diff)
			}
			if spy.gotKubeNil {
				t.Error("expected the callbacks to pass their kube client through")
			}
		})
	}
}

func TestAPICallbacks_NonPrefixedNameUsedAsIs(t *testing.T) {
	// Other consumers of this package do not prefix their shadow names; the
	// resolver must be a no-op for them.
	for _, name := range []string{"SERVICE_MANAGER_INSTANCE", "my-binding"} {
		t.Run(name, func(t *testing.T) {
			spy := &saveSpy{}
			ac := NewAPICallbacks(&test.MockClient{}, spy.fn())

			if err := ac.Create(types.NamespacedName{Name: name}, true)(nil, context.Background()); err != nil {
				t.Fatalf("callback returned unexpected error: %v", err)
			}
			if spy.gotName.Name != name {
				t.Errorf("expected name %q to be used as-is, got %q", name, spy.gotName.Name)
			}
		})
	}
}

func TestAPICallbacks_NamespacedNamePreserved(t *testing.T) {
	spy := &saveSpy{}
	ac := NewAPICallbacks(&test.MockClient{}, spy.fn())

	if err := ac.Update(types.NamespacedName{Namespace: "ns", Name: "TF-x"}, true)(nil, context.Background()); err != nil {
		t.Fatalf("callback returned unexpected error: %v", err)
	}
	want := types.NamespacedName{Namespace: "ns", Name: "x"}
	if diff := cmp.Diff(want, spy.gotName); diff != "" {
		t.Errorf("resolved name mismatch (-want, +got):\n%s", diff)
	}
}

func TestAPICallbacks_ResolutionFailureIsLoud(t *testing.T) {
	notFound := kerrors.NewNotFound(schema.GroupResource{Group: "account.btp.sap.crossplane.io", Resource: "serviceinstances"}, "x")
	spy := &saveSpy{returnErr: notFound}
	sink := &logSink{}
	rec := &eventSpy{}

	ac := NewAPICallbacks(&test.MockClient{}, spy.fn(),
		WithCallbackLogger(logr.New(sink)),
		WithCallbackEventRecorder(rec, func(nn types.NamespacedName) resource.Managed {
			si := &v1alpha1.ServiceInstance{}
			si.SetName(nn.Name)
			return si
		}),
	)

	err := ac.Destroy(types.NamespacedName{Name: "TF-x"}, true)(nil, context.Background())
	if err == nil {
		t.Fatal("expected the callback to return an error when the result cannot be persisted")
	}
	// The error must name the RESOLVED managed resource, not the shadow.
	if !strings.Contains(err.Error(), "/x") || strings.Contains(err.Error(), "TF-x") {
		t.Errorf("expected the error to name the resolved resource, got: %v", err)
	}

	if len(sink.errors) != 1 {
		t.Errorf("expected exactly one error-level log entry, got %d (info entries: %d)", len(sink.errors), len(sink.infos))
	}
	if len(sink.infos) != 0 {
		t.Errorf("a dropped async result must not be logged at info level, got %v", sink.infos)
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected exactly one event, got %d", len(rec.events))
	}
	if rec.events[0].Type != event.TypeWarning {
		t.Errorf("expected a Warning event, got %q", rec.events[0].Type)
	}
}

func TestAPICallbacks_NoRecorderIsSafe(t *testing.T) {
	spy := &saveSpy{returnErr: kerrors.NewNotFound(schema.GroupResource{Resource: "serviceinstances"}, "x")}
	ac := NewAPICallbacks(&test.MockClient{}, spy.fn(), WithCallbackLogger(logr.New(&logSink{})))

	if err := ac.Create(types.NamespacedName{Name: "TF-x"}, true)(nil, context.Background()); err == nil {
		t.Fatal("expected an error when the result cannot be persisted")
	}
}

// applyFailedWithDetail builds a genuine upjet apply failure out of a
// terraform CLI JSON log line, which is what the workspace hands the callbacks
// in production. The resulting error message is "apply failed: <summary>: <detail>".
func applyFailedWithDetail(t *testing.T, summary, detail string) error {
	t.Helper()
	line, err := json.Marshal(tferrors.TerraformLog{
		Level:      "error",
		Message:    summary,
		Diagnostic: tferrors.LogDiagnostic{Severity: "error", Summary: summary, Detail: detail},
	})
	if err != nil {
		t.Fatalf("cannot build terraform log fixture: %v", err)
	}
	return tferrors.NewApplyFailed(line)
}

func TestAPICallbacks_BoundsConditionMessage(t *testing.T) {
	t.Run("LongMessageTruncatedFromTheTail", func(t *testing.T) {
		// A conflict marker in the head must survive: the ServiceInstance
		// controller detects create conflicts by matching on it.
		applyErr := applyFailedWithDetail(t, "Conflict: resource already exists", strings.Repeat("x", 64*1024))
		spy := &saveSpy{}
		ac := NewAPICallbacks(&test.MockClient{}, spy.fn())

		if err := ac.Create(types.NamespacedName{Name: "TF-x"}, true)(applyErr, context.Background()); err != nil {
			t.Fatalf("callback returned unexpected error: %v", err)
		}

		msg := spy.gotConds[0].Message
		if len(msg) >= len(applyErr.Error()) {
			t.Fatalf("expected the message to be truncated, got %d bytes for a %d byte input", len(msg), len(applyErr.Error()))
		}
		if !strings.HasPrefix(msg, applyErr.Error()[:defaultMaxConditionMessageBytes]) {
			t.Error("truncation must preserve the head of the message byte-for-byte")
		}
		if !strings.Contains(msg, "Conflict") {
			t.Error("truncation must preserve the head of the message, where the terraform error class appears")
		}
		if !strings.Contains(msg, "truncated") {
			t.Error("expected a truncation marker in the bounded message")
		}
	})

	t.Run("ShortMessageUnchanged", func(t *testing.T) {
		spy := &saveSpy{}
		ac := NewAPICallbacks(&test.MockClient{}, spy.fn())

		applyErr := applyFailedWithDetail(t, "Conflict: resource already exists", "boom")
		if err := ac.Create(types.NamespacedName{Name: "TF-x"}, true)(applyErr, context.Background()); err != nil {
			t.Fatalf("callback returned unexpected error: %v", err)
		}
		if got, want := spy.gotConds[0].Message, applyErr.Error(); got != want {
			t.Errorf("expected the short message to be byte-identical\nwant: %q\ngot:  %q", want, got)
		}
	})

	t.Run("BoundDisabled", func(t *testing.T) {
		spy := &saveSpy{}
		ac := NewAPICallbacks(&test.MockClient{}, spy.fn(), WithMaxConditionMessageBytes(0))

		applyErr := applyFailedWithDetail(t, "apply error", strings.Repeat("y", 32*1024))
		if err := ac.Create(types.NamespacedName{Name: "TF-x"}, true)(applyErr, context.Background()); err != nil {
			t.Fatalf("callback returned unexpected error: %v", err)
		}
		if got, want := spy.gotConds[0].Message, applyErr.Error(); got != want {
			t.Errorf("expected no truncation when the bound is disabled: got %d bytes, want %d", len(got), len(want))
		}
	})
}

func TestAPICallbacks_WithNameResolver(t *testing.T) {
	spy := &saveSpy{}
	ac := NewAPICallbacks(&test.MockClient{}, spy.fn(), WithNameResolver(func(nn types.NamespacedName) types.NamespacedName {
		return types.NamespacedName{Name: "override"}
	}))

	if err := ac.Create(types.NamespacedName{Name: "TF-x"}, true)(nil, context.Background()); err != nil {
		t.Fatalf("callback returned unexpected error: %v", err)
	}
	if spy.gotName.Name != "override" {
		t.Errorf("expected the injected resolver to be used, got %q", spy.gotName.Name)
	}
}

func TestStripShadowPrefix(t *testing.T) {
	cases := map[string]struct {
		in   types.NamespacedName
		want types.NamespacedName
	}{
		"Prefixed":             {in: types.NamespacedName{Name: "TF-x"}, want: types.NamespacedName{Name: "x"}},
		"PrefixOnly":           {in: types.NamespacedName{Name: "TF-"}, want: types.NamespacedName{Name: ""}},
		"DoublePrefixOnlyOnce": {in: types.NamespacedName{Name: "TF-TF-x"}, want: types.NamespacedName{Name: "TF-x"}},
		"PrefixNotAtStart":     {in: types.NamespacedName{Name: "notTF-x"}, want: types.NamespacedName{Name: "notTF-x"}},
		"NoPrefix":             {in: types.NamespacedName{Name: "my-binding"}, want: types.NamespacedName{Name: "my-binding"}},
		"NamespacePreserved":   {in: types.NamespacedName{Namespace: "ns", Name: "TF-x"}, want: types.NamespacedName{Namespace: "ns", Name: "x"}},
		"EmptyName":            {in: types.NamespacedName{}, want: types.NamespacedName{}},
		"CaseSensitivePrefix":  {in: types.NamespacedName{Name: "tf-x"}, want: types.NamespacedName{Name: "tf-x"}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, StripShadowPrefix(tc.in)); diff != "" {
				t.Errorf("StripShadowPrefix(...) mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}
