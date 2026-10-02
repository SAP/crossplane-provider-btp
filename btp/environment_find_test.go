package btp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sap/crossplane-provider-btp/internal"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

// newTestClientListing serves list as the subaccount's environment instances,
// or a 500 when list is nil.
func newTestClientListing(t *testing.T, list []provisioningclient.BusinessEnvironmentInstanceResponseObject) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if list == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body := provisioningclient.BusinessEnvironmentInstancesResponseCollection{EnvironmentInstances: list}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("encode list: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := provisioningclient.NewConfiguration()
	cfg.HTTPClient = srv.Client()
	cfg.Servers = provisioningclient.ServerConfigurations{{URL: srv.URL}}
	return &Client{ProvisioningServiceClient: provisioningclient.NewAPIClient(cfg).EnvironmentsAPI}
}

func environment(id, envType, parameters string) provisioningclient.BusinessEnvironmentInstanceResponseObject {
	return provisioningclient.BusinessEnvironmentInstanceResponseObject{
		Id:              internal.Ptr(id),
		EnvironmentType: internal.Ptr(envType),
		Parameters:      internal.Ptr(parameters),
	}
}

func TestFindEnvironment(t *testing.T) {
	tests := []struct {
		name      string
		envType   EnvironmentType
		list      []provisioningclient.BusinessEnvironmentInstanceResponseObject
		wantID    string
		wantFound bool
		wantErr   bool
	}{
		{
			name:    "No match",
			envType: KymaEnvironmentType(),
			list:    []provisioningclient.BusinessEnvironmentInstanceResponseObject{environment("other", "kyma", `{"name":"other"}`)},
		},
		{
			name:    "Kyma matches on the name parameter",
			envType: KymaEnvironmentType(),
			list: []provisioningclient.BusinessEnvironmentInstanceResponseObject{
				environment("other", "kyma", `{"name":"other"}`),
				environment("kyma-1", "kyma", `{"name":"wanted"}`),
			},
			wantID:    "kyma-1",
			wantFound: true,
		},
		{
			name:      "Cloud Foundry matches on the instance_name parameter",
			envType:   CloudFoundryEnvironmentType(),
			list:      []provisioningclient.BusinessEnvironmentInstanceResponseObject{environment("cf-1", "cloudfoundry", `{"instance_name":"wanted"}`)},
			wantID:    "cf-1",
			wantFound: true,
		},
		{
			name:    "Same name of another type is ignored",
			envType: KymaEnvironmentType(),
			list:    []provisioningclient.BusinessEnvironmentInstanceResponseObject{environment("cf-1", "cloudfoundry", `{"name":"wanted"}`)},
		},
		{
			// Matching the legacy CF key on Kyma would widen the match.
			name:    "Another type's parameter key is ignored",
			envType: KymaEnvironmentType(),
			list:    []provisioningclient.BusinessEnvironmentInstanceResponseObject{environment("kyma-1", "kyma", `{"instance_name":"wanted"}`)},
		},
		{
			// One environment with malformed parameters must not hide a valid match.
			name:    "Unparsable parameters are skipped",
			envType: KymaEnvironmentType(),
			list: []provisioningclient.BusinessEnvironmentInstanceResponseObject{
				environment("broken", "kyma", `not json`),
				environment("kyma-1", "kyma", `{"name":"wanted"}`),
			},
			wantID:    "kyma-1",
			wantFound: true,
		},
		{
			name:    "Ambiguous match is an error",
			envType: KymaEnvironmentType(),
			list: []provisioningclient.BusinessEnvironmentInstanceResponseObject{
				environment("kyma-1", "kyma", `{"name":"wanted"}`),
				environment("kyma-2", "kyma", `{"name":"wanted"}`),
			},
			wantErr: true,
		},
		{
			// A failed list must never read as not-found, or the caller would create a duplicate.
			name:    "List failure is an error",
			envType: KymaEnvironmentType(),
			list:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClientListing(t, tt.list)

			got, found, err := c.FindEnvironment(context.Background(), tt.envType, "wanted")
			if (err != nil) != tt.wantErr {
				t.Errorf("FindEnvironment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if found != tt.wantFound || got.GetId() != tt.wantID {
				t.Errorf("FindEnvironment() = (%q, %v), want (%q, %v)", got.GetId(), found, tt.wantID, tt.wantFound)
			}
		})
	}
}

func TestFindCloudFoundryEnvironment(t *testing.T) {
	tests := []struct {
		name      string
		list      []provisioningclient.BusinessEnvironmentInstanceResponseObject
		wantID    string
		wantFound bool
		wantErr   bool
	}{
		{
			name: "No Cloud Foundry environment",
			list: []provisioningclient.BusinessEnvironmentInstanceResponseObject{environment("kyma-1", "kyma", `{"name":"k"}`)},
		},
		{
			// A subaccount holds at most one, so it is found by type whatever its names.
			name: "The one environment is found by type",
			list: []provisioningclient.BusinessEnvironmentInstanceResponseObject{
				environment("kyma-1", "kyma", `{"name":"k"}`),
				environment("cf-1", "cloudfoundry", `{"instance_name":"any-org"}`),
			},
			wantID:    "cf-1",
			wantFound: true,
		},
		{
			name: "More than one is an error",
			list: []provisioningclient.BusinessEnvironmentInstanceResponseObject{
				environment("cf-1", "cloudfoundry", `{}`),
				environment("cf-2", "cloudfoundry", `{}`),
			},
			wantErr: true,
		},
		{
			name:    "List failure is an error",
			list:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClientListing(t, tt.list)

			got, found, err := c.FindCloudFoundryEnvironment(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("FindCloudFoundryEnvironment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if found != tt.wantFound || got.GetId() != tt.wantID {
				t.Errorf("FindCloudFoundryEnvironment() = (%q, %v), want (%q, %v)", got.GetId(), found, tt.wantID, tt.wantFound)
			}
		})
	}
}
