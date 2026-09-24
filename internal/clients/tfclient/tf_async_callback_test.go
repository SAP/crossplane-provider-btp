package tfclient

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestAPICallbacks_ReturnsSaveError(t *testing.T) {
	boom := errors.New("cannot write status")
	ac := NewAPICallbacks(nil, func(context.Context, client.Client, types.NamespacedName, ...xpv1.Condition) error {
		return boom
	})

	err := ac.Destroy(types.NamespacedName{Name: "TF-test-instance"}, false)(errors.New("async delete failed"), context.Background())
	if !errors.Is(err, boom) {
		t.Errorf("expected the save error to be returned, got %v", err)
	}
}
