package mrstatus

import (
	"strings"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeobj "k8s.io/apimachinery/pkg/runtime"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

// recorderFake collects the events emitted through the event.Recorder
// interface so RecordOnChange's edge-triggering can be asserted.
type recorderFake struct {
	events []event.Event
}

func (r *recorderFake) Event(_ runtimeobj.Object, e event.Event) {
	r.events = append(r.events, e)
}

func (r *recorderFake) WithAnnotations(_ ...string) event.Recorder { return r }

func TestExternalResourceFailed(t *testing.T) {
	got := ExternalResourceFailed("BTP says no", 7)

	if got.Type != xpv1.TypeReady {
		t.Errorf("expected the Ready condition type, got %q", got.Type)
	}
	if got.Status != corev1.ConditionFalse {
		t.Errorf("expected Ready=False, got %q", got.Status)
	}
	if got.Reason != ReasonExternalResourceFailed {
		t.Errorf("expected reason %q, got %q", ReasonExternalResourceFailed, got.Reason)
	}
	if got.Message != "BTP says no" {
		t.Errorf("expected the message to be carried verbatim, got %q", got.Message)
	}
	if got.ObservedGeneration != 7 {
		t.Errorf("expected ObservedGeneration 7, got %d", got.ObservedGeneration)
	}
	if got.LastTransitionTime.IsZero() {
		t.Error("expected LastTransitionTime to be stamped")
	}
}

func TestTruncate(t *testing.T) {
	cases := map[string]struct {
		in            string
		max           int
		wantUnchanged bool
		wantHead      string
	}{
		"ShorterThanMax":                {in: "abc", max: 10, wantUnchanged: true},
		"ExactlyMax":                    {in: "abcde", max: 5, wantUnchanged: true},
		"Empty":                         {in: "", max: 5, wantUnchanged: true},
		"MaxZeroDisablesTruncation":     {in: strings.Repeat("a", 100), max: 0, wantUnchanged: true},
		"NegativeMaxDisablesTruncation": {in: strings.Repeat("a", 100), max: -1, wantUnchanged: true},
		"OneOverMax":                    {in: "abcdef", max: 5, wantHead: "abcde"},
		"Long":                          {in: strings.Repeat("a", 100), max: 10, wantHead: strings.Repeat("a", 10)},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Truncate(tc.in, tc.max)

			if tc.wantUnchanged {
				if got != tc.in {
					t.Errorf("expected the input to be returned unchanged, got %q", got)
				}
				return
			}
			if !strings.HasPrefix(got, tc.wantHead) {
				t.Errorf("expected the head %q to be preserved, got %q", tc.wantHead, got)
			}
			if len(got) <= tc.max {
				t.Errorf("expected the marker to be appended after the head, got %q", got)
			}
			if !strings.Contains(got, "truncated") {
				t.Errorf("expected a truncation marker, got %q", got)
			}
		})
	}
}

func TestTruncateMarkerReportsDroppedBytes(t *testing.T) {
	got := Truncate(strings.Repeat("a", 100), 10)
	if !strings.HasSuffix(got, "[truncated 90 bytes]") {
		t.Errorf("expected the marker to name the dropped byte count, got %q", got)
	}
}

func TestDeleteFailed(t *testing.T) {
	since := metav1.NewTime(time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC))

	got := DeleteFailed(since, "BTP refused the deprovision")

	if got.Type != TypeExternalDeletion {
		t.Errorf("expected the dedicated %q type (Ready would be overwritten by Deleting()), got %q",
			TypeExternalDeletion, got.Type)
	}
	if got.Type == xpv1.TypeReady {
		t.Error("the delete-failure condition must never use the Ready type")
	}
	if got.Status != corev1.ConditionFalse {
		t.Errorf("expected status False, got %q", got.Status)
	}
	if got.Reason != ReasonDeleteFailed {
		t.Errorf("expected reason %q, got %q", ReasonDeleteFailed, got.Reason)
	}
	if !strings.Contains(got.Message, "2026-08-05T12:00:00Z") {
		t.Errorf("expected the deletion timestamp in the message, got %q", got.Message)
	}
	if !strings.Contains(got.Message, "BTP refused the deprovision") {
		t.Errorf("expected the last error in the message, got %q", got.Message)
	}
}

func TestDeleteFailedMessageIsByteStable(t *testing.T) {
	since := metav1.NewTime(time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC))

	first := DeleteFailed(since, "same error").Message
	time.Sleep(time.Millisecond)
	second := DeleteFailed(since, "same error").Message

	if first != second {
		t.Errorf("the message must not depend on wall-clock time, got %q then %q", first, second)
	}
}

func TestDeleteFailedMessageIsBounded(t *testing.T) {
	since := metav1.NewTime(time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC))

	got := DeleteFailed(since, strings.Repeat("x", MaxMessageBytes*4)).Message

	if !strings.Contains(got, "truncated") {
		t.Errorf("expected an over-long terraform error to be truncated, got %d bytes", len(got))
	}
}

func TestRecordOnChange(t *testing.T) {
	since := metav1.NewTime(time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC))
	cr := &v1alpha1.ServiceInstance{}
	rec := &recorderFake{}

	// First observation of the failure: condition set, one Warning event.
	if emitted := RecordOnChange(rec, cr, DeleteFailed(since, "boom"), EventReasonDeleteFailed); !emitted {
		t.Error("expected an event on the first observation of a failure")
	}
	if got := cr.GetCondition(TypeExternalDeletion); got.Reason != ReasonDeleteFailed {
		t.Errorf("expected the condition to be set, got %+v", got)
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected exactly 1 event, got %d", len(rec.events))
	}
	if rec.events[0].Type != event.TypeWarning {
		t.Errorf("expected a Warning event, got %q", rec.events[0].Type)
	}
	if rec.events[0].Reason != EventReasonDeleteFailed {
		t.Errorf("expected reason %q, got %q", EventReasonDeleteFailed, rec.events[0].Reason)
	}

	// Same failure again: no further event. This is the whole point - the
	// deprovision is retried every reconcile and must not spam the event log.
	if emitted := RecordOnChange(rec, cr, DeleteFailed(since, "boom"), EventReasonDeleteFailed); emitted {
		t.Error("expected no event for an unchanged failure")
	}
	if len(rec.events) != 1 {
		t.Errorf("expected the event count to stay at 1, got %d", len(rec.events))
	}

	// A different failure is a new edge.
	if emitted := RecordOnChange(rec, cr, DeleteFailed(since, "different boom"), EventReasonDeleteFailed); !emitted {
		t.Error("expected an event when the failure changes")
	}
	if len(rec.events) != 2 {
		t.Errorf("expected 2 events, got %d", len(rec.events))
	}
	if got := cr.GetCondition(TypeExternalDeletion); !strings.Contains(got.Message, "different boom") {
		t.Errorf("expected the condition to carry the new failure, got %q", got.Message)
	}
}

func TestRecordOnChangeNilRecorderIsSafe(t *testing.T) {
	since := metav1.NewTime(time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC))
	cr := &v1alpha1.ServiceInstance{}

	if emitted := RecordOnChange(nil, cr, DeleteFailed(since, "boom"), EventReasonDeleteFailed); emitted {
		t.Error("expected no event to be reported with a nil recorder")
	}
	if got := cr.GetCondition(TypeExternalDeletion); got.Reason != ReasonDeleteFailed {
		t.Errorf("expected the condition to be set even without a recorder, got %+v", got)
	}
}
