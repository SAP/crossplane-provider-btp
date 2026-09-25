package tfclient

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	ujresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/crossplane/upjet/pkg/terraform"
	tferrors "github.com/crossplane/upjet/pkg/terraform/errors"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestAPICallbacks_RecordsTheOperation asserts that the recorded reason names
// the operation; upjet v1 reports create and update failures alike as ApplyFailed.
func TestAPICallbacks_RecordsTheOperation(t *testing.T) {
	applyFailed := tferrors.NewApplyFailed([]byte(`{"@level":"error","@message":"rejected"}`))

	cases := map[string]struct {
		callback func(*APICallbacks) terraform.CallbackFn
		err      error
		want     xpv1.ConditionReason
	}{
		"CreateFailure": {
			callback: func(ac *APICallbacks) terraform.CallbackFn { return ac.Create("TF-test-instance") },
			err:      applyFailed,
			want:     ujresource.ReasonAsyncCreateFailure,
		},
		"UpdateFailure": {
			callback: func(ac *APICallbacks) terraform.CallbackFn { return ac.Update("TF-test-instance") },
			err:      applyFailed,
			want:     ujresource.ReasonAsyncUpdateFailure,
		},
		"DestroyFailure": {
			callback: func(ac *APICallbacks) terraform.CallbackFn { return ac.Destroy("TF-test-instance") },
			err:      tferrors.NewDestroyFailed([]byte(`{"@level":"error","@message":"rejected"}`)),
			want:     ujresource.ReasonDestroyFailure,
		},
		"UpdateSuccess": {
			callback: func(ac *APICallbacks) terraform.CallbackFn { return ac.Update("TF-test-instance") },
			want:     ujresource.ReasonSuccess,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var got xpv1.Condition
			ac := NewAPICallbacks(nil, func(_ context.Context, _ client.Client, _ string, conditions ...xpv1.Condition) error {
				got = conditions[0]
				return nil
			})
			if err := tc.callback(ac)(tc.err, context.Background()); err != nil {
				t.Fatalf("callback failed: %v", err)
			}
			if got.Reason != tc.want {
				t.Errorf("expected reason %q, got %q (message %q)", tc.want, got.Reason, got.Message)
			}
		})
	}
}

func TestAPICallbacks_ReturnsSaveError(t *testing.T) {
	boom := errors.New("cannot write status")
	ac := NewAPICallbacks(nil, func(context.Context, client.Client, string, ...xpv1.Condition) error {
		return boom
	})

	err := ac.Destroy("TF-test-instance")(errors.New("async delete failed"), context.Background())
	if !errors.Is(err, boom) {
		t.Errorf("expected the save error to be returned, got %v", err)
	}
}
