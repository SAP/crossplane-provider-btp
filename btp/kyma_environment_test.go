package btp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

// newTestClientCapturingPayload spins up an httptest server that captures the create-environment
// request body and returns a Client wired to hit it. This exercises the real JSON serialization
// path, which is where issue #910 manifested: landscapeLabel was never put on the payload, so it
// was omitted from the wire request.
func newTestClientCapturingPayload(t *testing.T, captured *provisioningclient.CreateEnvironmentInstanceRequestPayload) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, captured); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		id := "created-id"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(provisioningclient.CreatedEnvironmentInstanceResponseObject{Id: &id})
	}))

	cfg := provisioningclient.NewConfiguration()
	cfg.HTTPClient = srv.Client()
	cfg.Servers = provisioningclient.ServerConfigurations{{URL: srv.URL}}
	api := provisioningclient.NewAPIClient(cfg).EnvironmentsAPI

	return &Client{ProvisioningServiceClient: api}, srv
}

func TestCreateKymaEnvironment_LandscapeLabel(t *testing.T) {
	landscape := "cf-eu12"
	tests := map[string]struct {
		landscapeLabel *string
		wantSet        bool
		wantValue      string
	}{
		"set on multi-landscape region": {landscapeLabel: &landscape, wantSet: true, wantValue: landscape},
		"unset on single-landscape region stays absent": {landscapeLabel: nil, wantSet: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var got provisioningclient.CreateEnvironmentInstanceRequestPayload
			c, srv := newTestClientCapturingPayload(t, &got)
			defer srv.Close()

			_, err := c.CreateKymaEnvironment(
				context.Background(), "inst", "plan",
				InstanceParameters{}, "uid", "user@example.com", tc.landscapeLabel,
			)
			if err != nil {
				t.Fatalf("CreateKymaEnvironment: %v", err)
			}

			val, ok := got.GetLandscapeLabelOk()
			if ok != tc.wantSet {
				t.Fatalf("landscapeLabel present=%v, want %v", ok, tc.wantSet)
			}
			if tc.wantSet && *val != tc.wantValue {
				t.Fatalf("landscapeLabel=%q, want %q", *val, tc.wantValue)
			}
		})
	}
}
