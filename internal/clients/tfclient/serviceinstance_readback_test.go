package tfclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tfprovider "github.com/SAP/terraform-provider-btp/btp/provider"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	xpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	upcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	"github.com/crossplane/upjet/v2/pkg/metrics"
	"github.com/crossplane/upjet/v2/pkg/terraform"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestServiceInstanceReadbackAfterCacheLoss(t *testing.T) {
	ctx := context.Background()
	const id = "11111111-1111-4111-8111-111111111111"
	const desired = `{"consumer_approval_policy":{"pipelines":{"metrics":{"auto_approve":true},"traces":{"auto_approve":true},"logs":{"auto_approve":true}}},"metadata":{"display_name":"12345678"},"count":2}`
	brokerParameters := map[string]any{"consumer_approval_policy": map[string]any{}, "metadata": map[string]any{"display_name": "BTP_AT_11111111-1111-4111-8111-111111111111"}, "count": 2}
	reads := 0
	updates := 0
	retrievable := true
	wrapped := false
	failureStatus := ""
	malformed := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(headerCLIBackendStatus, "200")
		if strings.HasPrefix(r.URL.Path, "/login") {
			w.Header().Set(headerCLISessionId, "test-session")
			_ = json.NewEncoder(w).Encode(map[string]string{"mail": "test@example.com", "issuer": "https://idp.example.com"})
			return
		}
		var command struct {
			Args map[string]any `json:"paramValues"`
		}
		if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
			t.Errorf("decode command: %v", err)
			w.WriteHeader(500)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/services/instance") {
			t.Errorf("unexpected command: %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		if r.URL.RawQuery == "update" {
			w.Header().Set(headerCLIBackendStatus, "202")
			parameters, ok := command.Args["parameters"].(string)
			if !ok || json.Unmarshal([]byte(parameters), &brokerParameters) != nil {
				t.Error("update did not send valid desired parameters")
			}
			updates++
		}
		if command.Args["parameters"] == "true" {
			reads++
			if !retrievable {
				w.Header().Set(headerCLIBackendStatus, "400")
				_, _ = w.Write([]byte(`{"error":"parameters are not retrievable"}`))
				return
			}
			if failureStatus != "" {
				w.Header().Set(headerCLIBackendStatus, failureStatus)
				_, _ = w.Write([]byte(`{"error":"unavailable"}`))
				return
			}
			if malformed {
				_, _ = w.Write([]byte(`not JSON`))
				return
			}
			if wrapped {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": brokerParameters})
			} else {
				_ = json.NewEncoder(w).Encode(brokerParameters)
			}
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": id, "name": "example", "subaccount_id": id,
			"service_plan_id": id, "ready": true, "usable": true,
			"last_operation": map[string]any{"state": "succeeded"},
		})
	}))
	defer srv.Close()
	transport := &cliTransport{base: srv.Client().Transport, evictSub: func(string) {}, evictAll: func() {}}
	p := newCachingProvider(tfprovider.NewWithClient(&http.Client{Transport: transport}))
	// Each connector has an empty operation store, just as a new provider
	// process does. The proxy object has desired parameters and no observation.
	observe := func(provider fwprovider.Provider, heal bool) (bool, error) {
		t.Helper()
		cr := &v1alpha1.SubaccountServiceInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "example", UID: "stable-uid", Annotations: map[string]string{"crossplane.io/external-name": id}},
		}
		cr.Spec.ForProvider.Name = ptrReadback("example")
		cr.Spec.ForProvider.SubaccountID = ptrReadback(id)
		cr.Spec.ForProvider.ServiceplanID = ptrReadback(id)
		cr.Spec.ForProvider.ServiceplanName = ptrReadback("standard")
		cr.Spec.ForProvider.ServiceOfferingName = ptrReadback("example-service")
		cr.Spec.ForProvider.Parameters = ptrReadback(desired)
		cr.Spec.ForProvider.Timeouts = &v1alpha1.TimeoutsParameters{Update: ptrReadback("1s")}
		connector := upcontroller.NewTerraformPluginFrameworkConnector(fake.NewClientBuilder().Build(),
			readbackSetup(provider, srv.URL), config.GetProvider().Resources["btp_subaccount_service_instance"], upcontroller.NewOperationStore(logging.NewNopLogger()),
			upcontroller.WithTerraformPluginFrameworkLogger(logging.NewNopLogger()),
			upcontroller.WithTerraformPluginFrameworkMetricRecorder(metrics.NewMetricRecorder(schema.GroupVersionKind{}, nil, time.Minute)))
		external, err := connector.Connect(ctx, cr)
		if err != nil {
			return false, err
		}
		observation, err := external.Observe(ctx, cr)
		if err != nil {
			return false, err
		}
		if heal && !observation.ResourceUpToDate {
			if _, err := external.Update(ctx, cr); err != nil {
				t.Fatal(err)
			}
		}
		return observation.ResourceUpToDate, nil
	}
	// Control reproduces the production failure with a successful plain response.
	if ok, err := observe(tfprovider.NewWithClient(srv.Client()), false); err != nil || !ok {
		t.Fatalf("control: upToDate=%v err=%v", ok, err)
	}
	if ok, err := observe(p, true); err != nil || ok {
		t.Fatalf("plain drift/update: upToDate=%v err=%v", ok, err)
	}
	if updates != 1 || brokerParameters["metadata"].(map[string]any)["display_name"] != "12345678" {
		t.Fatal("ordinary Update must heal the display name and policy")
	}
	assertPipelinePolicy(t, brokerParameters)
	if ok, err := observe(p, false); err != nil || !ok {
		t.Fatalf("verify after restart: %v %v", ok, err)
	}
	brokerParameters["extra"] = true
	brokerParameters["metadata"].(map[string]any)["broker_default"] = "retained"
	wrapped = true
	if ok, err := observe(p, false); err != nil || !ok {
		t.Fatalf("wrapped defaults: %v %v", ok, err)
	}
	wrapped = false
	brokerParameters["consumer_approval_policy"] = map[string]any{}
	if ok, err := observe(p, true); err != nil || ok {
		t.Fatalf("policy-only repair: %v %v", ok, err)
	}
	if updates != 2 {
		t.Fatalf("policy update count %d", updates)
	}
	assertPipelinePolicy(t, brokerParameters)
	if ok, err := observe(p, false); err != nil || !ok {
		t.Fatalf("policy verified: %v %v", ok, err)
	}
	brokerParameters = map[string]any{}
	if ok, err := observe(p, false); err != nil || ok {
		t.Fatalf("empty object is actual drift: %v %v", ok, err)
	}
	for _, status := range []string{"403", "429", "500"} {
		failureStatus = status
		if ok, err := observe(p, false); err == nil || ok {
			t.Fatalf("backend %s must not converge: %v %v", status, ok, err)
		}
	}
	failureStatus = ""
	malformed = true
	if ok, err := observe(p, false); err == nil || ok {
		t.Fatalf("malformed readback must not converge: %v %v", ok, err)
	}
	malformed = false
	retrievable = false
	if ok, err := observe(p, false); err == nil || ok {
		t.Fatalf("unavailable readback must not converge: %v %v", ok, err)
	}
	if ok, err := observe(&writeOnlyProvider{cachingProvider: p}, false); err != nil || !ok {
		t.Fatalf("explicit write-only mode: %v %v", ok, err)
	}
	if reads < 4 {
		t.Fatalf("missing readbacks: %d", reads)
	}

}

func assertPipelinePolicy(t *testing.T, parameters map[string]any) {
	t.Helper()
	pipelines := parameters["consumer_approval_policy"].(map[string]any)["pipelines"].(map[string]any)
	for _, pipeline := range []string{"metrics", "traces", "logs"} {
		if pipelines[pipeline].(map[string]any)["auto_approve"] != true {
			t.Fatalf("pipeline %s not applied", pipeline)
		}
	}
}

func ptrReadback(s string) *string { return &s }

func TestServiceInstanceReadbackOptionalInterfaces(t *testing.T) {
	p := newCachingProvider(tfprovider.New())
	for _, create := range p.Resources(context.Background()) {
		r := create()
		var metadata resource.MetadataResponse
		r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "btp"}, &metadata)
		if metadata.TypeName != "btp_subaccount_service_instance" {
			continue
		}
		if _, ok := r.(resource.ResourceWithConfigure); !ok {
			t.Fatal("lost Configure")
		}
		if _, ok := r.(resource.ResourceWithIdentity); !ok {
			t.Fatal("lost identity")
		}
		if _, ok := r.(resource.ResourceWithImportState); !ok {
			t.Fatal("lost import")
		}
		return
	}
	t.Fatal("service instance adapter not registered")
}

func TestConfiguredParametersMatch(t *testing.T) {
	for _, tc := range []struct {
		name, desired, observed string
		matches                 bool
	}{
		{"defaults", `{"a":{"b":true}}`, `{"a":{"b":true,"default":2},"other":1}`, true},
		{"missing", `{"a":{"b":true}}`, `{"a":{}}`, false},
		{"scalar", `{"a":2}`, `{"a":3}`, false},
		{"null", `{"a":null}`, `{"a":null}`, true},
		{"missing null", `{"a":null}`, `{}`, false},
		{"array", `{"a":[1,2]}`, `{"a":[2,1]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var desired, observed map[string]any
			_ = json.Unmarshal([]byte(tc.desired), &desired)
			_ = json.Unmarshal([]byte(tc.observed), &observed)
			if configuredParametersMatch(desired, observed) != tc.matches {
				t.Fatal("unexpected comparison")
			}
		})
	}
}

func TestParameterNumberComparison(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		equal bool
	}{
		{"2", "2.0", true},
		{"2e0", "2", true},
		{"9007199254740992", "9007199254740993", false},
	} {
		var a, b map[string]any
		if err := decodeParameterObject(`{"number":`+tc.a+`}`, &a); err != nil {
			t.Fatal(err)
		}
		if err := decodeParameterObject(`{"number":`+tc.b+`}`, &b); err != nil {
			t.Fatal(err)
		}
		if configuredParametersMatch(a, b) != tc.equal {
			t.Fatalf("%s vs %s", tc.a, tc.b)
		}
	}
}

func readbackSetup(p fwprovider.Provider, url string) terraform.SetupFn {
	return func(context.Context, client.Client, xpresource.Managed) (terraform.Setup, error) {
		return terraform.Setup{FrameworkProvider: p, Configuration: map[string]any{
			"username": "test", "password": "test", "globalaccount": "example", "cli_server_url": url,
		}}, nil
	}
}
