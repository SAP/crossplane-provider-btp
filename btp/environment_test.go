package btp

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/fakes"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

// TestGetEnvironmentInstanceByID covers how the not-found flag is derived from
// the raw HTTP response. The interesting case is a transport-level failure,
// where the generated client returns a nil *http.Response next to the error:
// dereferencing it there used to panic on every reconcile of the affected
// resource, and controller-runtime requeues unconditionally, so the resource
// could never be observed or deleted again.
func TestGetEnvironmentInstanceByID(t *testing.T) {
	transportErr := errors.New("Get \"https://example.invalid/environments\": dial tcp: lookup example.invalid: no such host")
	apiErr := errors.New("404 Not Found")
	instance := &provisioningclient.BusinessEnvironmentInstanceResponseObject{
		Id: internal.Ptr("some-instance-id"),
	}

	tests := []struct {
		name         string
		apiClient    provisioningclient.EnvironmentsAPI
		wantInstance *provisioningclient.BusinessEnvironmentInstanceResponseObject
		wantNotFound bool
		wantErr      error
	}{
		{
			name: "NilResponseOnTransportError",
			apiClient: &fakes.MockProvisioningServiceClientTransportError{
				TransportError: transportErr,
			},
			wantInstance: nil,
			wantNotFound: false,
			wantErr:      transportErr,
		},
		{
			name: "NotFound",
			apiClient: &fakes.MockProvisioningServiceClientWithGetByID{
				GetByIDStatusCode: http.StatusNotFound,
				GetByIDError:      apiErr,
			},
			wantInstance: nil,
			wantNotFound: true,
			wantErr:      apiErr,
		},
		{
			name: "Success",
			apiClient: &fakes.MockProvisioningServiceClientWithGetByID{
				GetByIDResponse:   instance,
				GetByIDStatusCode: http.StatusOK,
			},
			wantInstance: instance,
			wantNotFound: false,
			wantErr:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Client{ProvisioningServiceClient: tt.apiClient}

			// A panic here is the regression this test exists for.
			got, notFound, err := c.GetEnvironmentInstanceByID(context.Background(), "some-instance-id")

			if notFound != tt.wantNotFound {
				t.Errorf("GetEnvironmentInstanceByID() notFound = %v, want %v", notFound, tt.wantNotFound)
			}
			if tt.wantErr == nil && err != nil {
				t.Errorf("GetEnvironmentInstanceByID() unexpected error: %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("GetEnvironmentInstanceByID() error = %v, want it to wrap %v", err, tt.wantErr)
			}
			if got != tt.wantInstance {
				t.Errorf("GetEnvironmentInstanceByID() instance = %v, want %v", got, tt.wantInstance)
			}
		})
	}
}
