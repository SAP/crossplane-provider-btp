package pollinterval

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
)

func TestDriftReset(t *testing.T) {
	errBoom := errors.New("boom")
	notSynced := cond(xpv2.TypeSynced, corev1.ConditionFalse, xpv2.ReasonReconcileError, time.Hour, 1)

	cases := map[string]struct {
		synced    xpv2.Condition
		updateErr error
		refreshed bool
	}{
		"SuccessRefreshesSynced": {synced: synced(time.Hour), refreshed: true},
		"ErrorKeepsSynced":       {synced: synced(time.Hour), updateErr: errBoom},
		"NotSyncedUntouched":     {synced: notSynced},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mg := mr(1, "", ready(time.Hour), tc.synced)
			inner := managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) {
				return &managed.ExternalClientFns{
					UpdateFn: func(context.Context, resource.Managed) (managed.ExternalUpdate, error) {
						return managed.ExternalUpdate{}, tc.updateErr
					},
				}, nil
			})
			ec, err := WithDriftReset(inner).Connect(context.Background(), mg)
			if err != nil {
				t.Fatalf("Connect() error = %v", err)
			}

			before := time.Now().Truncate(time.Second)
			_, err = ec.Update(context.Background(), mg)
			after := time.Now()
			if !errors.Is(err, tc.updateErr) {
				t.Fatalf("Update() error = %v, want %v", err, tc.updateErr)
			}

			got := mg.GetCondition(xpv2.TypeSynced)
			if tc.refreshed {
				if ltt := got.LastTransitionTime.Time; ltt.Before(before) || ltt.After(after) {
					t.Errorf("Synced LTT = %v, want in [%v, %v]", ltt, before, after)
				}
				got.LastTransitionTime = tc.synced.LastTransitionTime
			}
			if diff := cmp.Diff(tc.synced, got); diff != "" {
				t.Errorf("Synced (ignoring refreshed LTT): -want +got:\n%s", diff)
			}
			if r, want := mg.GetCondition(xpv2.TypeReady), ready(time.Hour); !r.LastTransitionTime.Equal(&want.LastTransitionTime) {
				t.Errorf("Ready LTT = %v, want unchanged", r.LastTransitionTime)
			}
		})
	}
}
