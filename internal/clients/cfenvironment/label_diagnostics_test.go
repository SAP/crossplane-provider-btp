package environments

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/go-logr/logr/funcr"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/fakes"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestCloudFoundryLabelDiagnostics(t *testing.T) {
	invalid := "not-json"
	tests := []struct {
		name                                    string
		labels                                  *string
		wantKeys                                []string
		wantValid, wantRecognized, wantNonEmpty bool
	}{
		{name: "nil labels", wantValid: true},
		{name: "invalid JSON", labels: &invalid},
		{name: "unknown key", labels: internal.Ptr(`{"Other":"secret"}`), wantKeys: []string{"Other"}, wantValid: true},
		{name: "endpoint variants sorted", labels: internal.Ptr(`{"z":"x","API Endpoint:":"","API Endpoint":"url"}`), wantKeys: []string{"API Endpoint", "API Endpoint:", "z"}, wantValid: true, wantRecognized: true, wantNonEmpty: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys, valid, recognized, nonEmpty := cloudFoundryLabelDiagnostics(tt.labels)
			if strings.Join(keys, ",") != strings.Join(tt.wantKeys, ",") || valid != tt.wantValid || recognized != tt.wantRecognized || nonEmpty != tt.wantNonEmpty {
				t.Fatalf("diagnostics = (%v, %v, %v, %v), want (%v, %v, %v, %v)", keys, valid, recognized, nonEmpty, tt.wantKeys, tt.wantValid, tt.wantRecognized, tt.wantNonEmpty)
			}
		})
	}
}

func TestCloudFoundryObservationDiagnostics(t *testing.T) {
	const errorMarker = "synthetic-error-marker"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized","error_description":"` + errorMarker + `"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"links": map[string]any{
			"login":   map[string]string{"href": "http://" + r.Host},
			"uaa":     map[string]string{"href": "http://" + r.Host},
			"app_ssh": map[string]any{"meta": map[string]string{"oauth_client": "ssh"}},
		}})
	}))
	t.Cleanup(server.Close)
	labels := `{"Org ID":"org-id","Org Name":"org","API Endpoint":"` + server.URL + `"}`
	env := &provisioningclient.BusinessEnvironmentInstanceResponseObject{Labels: &labels}
	fake := &fakes.MockProvisioningServiceClient{ApiResponse: &provisioningclient.BusinessEnvironmentInstancesResponseCollection{EnvironmentInstances: []provisioningclient.BusinessEnvironmentInstanceResponseObject{*env}}}
	client := NewCloudFoundryOrganization(btp.Client{
		Credential:                &btp.Credentials{UserCredential: &btp.UserCredential{Username: "synthetic-user", Password: "synthetic-password"}},
		ProvisioningServiceClient: fake,
	})
	cr := v1alpha1.CloudFoundryEnvironment{ObjectMeta: metav1.ObjectMeta{}}
	meta.SetExternalName(&cr, "12345678-1234-1234-1234-123456789012")

	var records []string
	logger := funcr.NewJSON(func(record string) { records = append(records, record) }, funcr.Options{})
	ctx := log.IntoContext(context.Background(), logger)
	gotEnvironment, gotManagers, err := client.DescribeInstance(ctx, cr)
	if err == nil || !strings.Contains(err.Error(), errorMarker) {
		t.Fatalf("DescribeInstance error = %v, want error containing %q", err, errorMarker)
	}
	if gotEnvironment != nil || gotManagers != nil {
		t.Fatalf("DescribeInstance returned environment/managers on failure: %v / %v", gotEnvironment, gotManagers)
	}
	if len(records) != 1 {
		t.Fatalf("logged %d records, want 1: %v", len(records), records)
	}
	for _, marker := range []string{server.URL, errorMarker, `"error"`} {
		if strings.Contains(records[0], marker) {
			t.Errorf("diagnostic log leaked %q: %s", marker, records[0])
		}
	}
	for _, expected := range []string{`"labelKeys":["API Endpoint","Org ID","Org Name"]`, `"labelsJSONValid":true`, `"apiEndpointLabelRecognized":true`, `"apiEndpointLabelNonEmpty":true`} {
		if !strings.Contains(records[0], expected) {
			t.Errorf("diagnostic log missing %q: %s", expected, records[0])
		}
	}
}
