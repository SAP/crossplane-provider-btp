package mrstatus

import (
	"strings"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
)

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
