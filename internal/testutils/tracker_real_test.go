package testutils

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
)

func TestNewRealResourceTracker(t *testing.T) {
	t.Run("InUse", func(t *testing.T) {
		cr := &v1alpha1.Subaccount{ObjectMeta: metav1.ObjectMeta{Name: "x", UID: "uid-1"}}
		tr := NewRealResourceTracker(t, cr, true)
		tr.SetConditions(context.Background(), cr)
		if got := cr.GetCondition(providerv1alpha1.UseCondition).Reason; got != providerv1alpha1.InUseReason {
			t.Fatalf("Reason = %q, want %q", got, providerv1alpha1.InUseReason)
		}
	})
	t.Run("NotInUse", func(t *testing.T) {
		cr := &v1alpha1.Subaccount{ObjectMeta: metav1.ObjectMeta{Name: "y", UID: "uid-2"}}
		tr := NewRealResourceTracker(t, cr, false)
		tr.SetConditions(context.Background(), cr)
		if got := cr.GetCondition(providerv1alpha1.UseCondition).Reason; got != providerv1alpha1.NotInUseReason {
			t.Fatalf("Reason = %q, want %q", got, providerv1alpha1.NotInUseReason)
		}
	})
}
