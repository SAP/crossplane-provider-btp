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

func TestCloudFoundryObservationDiagnosticsMalformedURL(t *testing.T) {
	const endpointMarker = "endpoint-marker.example"
	labels := `{"Org ID":"org-id","Org Name":"org","API Endpoint":"https://` + endpointMarker + `/%zz"}`
	records, environment, managers, err := observeCloudFoundryLabels(t, labels, &btp.Credentials{UserCredential: &btp.UserCredential{Username: "synthetic-user", Password: "synthetic-password"}})
	if err == nil || !strings.Contains(err.Error(), "%zz") {
		t.Fatalf("DescribeInstance error = %v, want malformed URL error", err)
	}
	assertObservationFailureDiagnostics(t, records, environment, managers, endpointMarker)
}

func TestCloudFoundryObservationDiagnosticsNonOAuthResponseBody(t *testing.T) {
	const errorMarker = "synthetic-api-response-marker"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"synthetic-token","token_type":"bearer","expires_in":3600}`))
			return
		}
		if r.URL.Path == "/v3/roles" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(errorMarker))
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
	records, environment, managers, err := observeCloudFoundryLabels(t, labels, &btp.Credentials{UserCredential: &btp.UserCredential{Username: "synthetic-user", Password: "synthetic-password"}})
	// The CF client deliberately reports only the HTTP status for this failure; it does not propagate the response body.
	if err == nil || !strings.Contains(err.Error(), "HTTP error (500)") || strings.Contains(err.Error(), errorMarker) {
		t.Fatalf("DescribeInstance error = %v, want generic HTTP 500 error without response body", err)
	}
	assertObservationFailureDiagnostics(t, records, environment, managers, errorMarker)
}

func TestCloudFoundryOrganizationDescribeInstanceConflictingLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels string
		keys   []string
	}{
		{name: "organization ID", labels: `{"Org ID":"id-marker-one","Org ID:":"id-marker-two"}`, keys: []string{"Org ID", "Org ID:"}},
		{name: "API endpoint", labels: `{"API Endpoint":"endpoint-marker-one","API Endpoint:":"endpoint-marker-two"}`, keys: []string{"API Endpoint", "API Endpoint:"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records, environment, managers, err := observeCloudFoundryLabels(t, tt.labels, nil)
			if err == nil || !strings.Contains(err.Error(), "conflicting Cloud Foundry label values") {
				t.Fatalf("DescribeInstance error = %v, want label conflict", err)
			}
			for _, key := range tt.keys {
				if !strings.Contains(err.Error(), key) {
					t.Errorf("error %q does not identify conflicting key %q", err, key)
				}
			}
			for _, marker := range []string{"id-marker-one", "id-marker-two", "endpoint-marker-one", "endpoint-marker-two"} {
				if strings.Contains(err.Error(), marker) {
					t.Errorf("error leaked label value %q: %v", marker, err)
				}
			}
			assertObservationFailureDiagnostics(t, records, environment, managers, "marker")
		})
	}
}

func observeCloudFoundryLabels(t *testing.T, labels string, credentials *btp.Credentials) ([]string, *provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, error) {
	t.Helper()
	env := &provisioningclient.BusinessEnvironmentInstanceResponseObject{Labels: &labels}
	fake := &fakes.MockProvisioningServiceClient{ApiResponse: &provisioningclient.BusinessEnvironmentInstancesResponseCollection{EnvironmentInstances: []provisioningclient.BusinessEnvironmentInstanceResponseObject{*env}}}
	client := NewCloudFoundryOrganization(btp.Client{Credential: credentials, ProvisioningServiceClient: fake})
	cr := v1alpha1.CloudFoundryEnvironment{ObjectMeta: metav1.ObjectMeta{}}
	meta.SetExternalName(&cr, "12345678-1234-1234-1234-123456789012")
	var records []string
	ctx := log.IntoContext(context.Background(), funcr.NewJSON(func(record string) { records = append(records, record) }, funcr.Options{}))
	environment, managers, err := client.DescribeInstance(ctx, cr)
	return records, environment, managers, err
}

func assertObservationFailureDiagnostics(t *testing.T, records []string, environment *provisioningclient.BusinessEnvironmentInstanceResponseObject, managers []v1alpha1.User, marker string) {
	t.Helper()
	if environment != nil || managers != nil {
		t.Fatalf("DescribeInstance returned environment/managers on failure: %v / %v", environment, managers)
	}
	if len(records) != 1 {
		t.Fatalf("logged %d records, want 1: %v", len(records), records)
	}
	for _, forbidden := range []string{marker, `"error"`} {
		if strings.Contains(records[0], forbidden) {
			t.Errorf("diagnostic log leaked %q: %s", forbidden, records[0])
		}
	}
}
