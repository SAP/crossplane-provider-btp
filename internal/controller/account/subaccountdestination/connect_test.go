package subaccountdestination

import (
	"context"
	"strings"
	"testing"

	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/account/destination"
	"github.com/sap/crossplane-provider-btp/internal/clients/account/destination/destinationtest"
	tracking_test "github.com/sap/crossplane-provider-btp/internal/tracking/test"
)

func TestConnect_BindingCheck(t *testing.T) {
	connect := func(t *testing.T, svc *destinationtest.Service, subaccountID string) (*external, error) {
		t.Helper()
		c := &connector{
			kube:            kubefake.NewClientBuilder().WithObjects(svc.BindingSecret("binding", "ns")).Build(),
			usage:           tracking_test.NoOpLegacyTracker{},
			resourcetracker: tracking_test.NoOpReferenceResolverTracker{},
		}
		cr := &v1alpha1.SubaccountDestination{Spec: v1alpha1.SubaccountDestinationSpec{ForProvider: v1alpha1.SubaccountDestinationParameters{
			Name:         "d",
			SubaccountID: &subaccountID,
			DestinationServiceBindingSecretRef: &xpv1.SecretKeySelector{
				SecretReference: xpv1.SecretReference{Name: "binding", Namespace: "ns"},
				Key:             destinationtest.BindingKey,
			},
		}}}
		ext, err := c.Connect(context.Background(), cr)
		if err != nil {
			return nil, err
		}
		return ext.(*external), nil
	}

	t.Run("binding of another subaccount", func(t *testing.T) {
		if _, err := connect(t, destinationtest.New(t, "sub-b"), "sub-a"); err == nil || !strings.Contains(err.Error(), "belongs to subaccount sub-b") {
			t.Fatalf("err = %v, want binding mismatch", err)
		}
	})
	t.Run("check and client share one token", func(t *testing.T) {
		svc := destinationtest.New(t, "sub-a")
		e, err := connect(t, svc, "sub-a")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := e.client.Get(context.Background(), "d"); !destination.IsNotFound(err) {
			t.Fatalf("Get: %v, want not found", err)
		}
		if n := svc.TokenRequests(); n != 1 {
			t.Errorf("token requests = %d, want 1", n)
		}
	})
}
