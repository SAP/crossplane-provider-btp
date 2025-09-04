package servicebindingnative

import(
	"context"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	servicemanagerclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

func TestServiceBindingClient_toCreateApiPayload(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for receiver constructor.
		client  *servicemanagerclient.APIClient
		cr      *v1alpha1.ServiceBinding
		kube    client.Client
		want    servicemanagerclient.CreateServiceBindingRequestPayload
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewServiceBindingClient(tt.client, tt.cr, tt.kube)
			got, gotErr := d.toCreateApiPayload(context.Background())
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("toCreateApiPayload() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("toCreateApiPayload() succeeded unexpectedly")
			}
			// TODO: update the condition below to compare got with tt.want.
			if true {
				t.Errorf("toCreateApiPayload() = %v, want %v", got, tt.want)
			}
		})
	}
}

