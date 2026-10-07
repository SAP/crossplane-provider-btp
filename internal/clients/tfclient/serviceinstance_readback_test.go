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
	const desired = `{"policy":{"enabled":true},"count":2}`
	brokerParameters := map[string]any{"policy": map[string]any{}, "count": 2}
	reads := 0
	updates := 0
	retrievable := true
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
				_, _ = w.Write([]byte(`{}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": brokerParameters})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": id, "name": "example", "subaccount_id": id,
			"service_plan_id": id, "ready": true, "usable": true,
			"last_operation": map[string]any{"state": "succeeded"},
		})
	}))
	defer srv.Close()
	p := newCachingProvider(tfprovider.NewWithClient(srv.Client()))
	// Each connector has an empty operation store, just as a new provider
	// process does. The proxy object has desired parameters and no observation.
	observe := func(provider fwprovider.Provider, heal bool) bool {
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
			t.Fatal(err)
		}
		observation, err := external.Observe(ctx, cr)
		if err != nil {
			t.Fatal(err)
		}
		if heal && !observation.ResourceUpToDate {
			if _, err := external.Update(ctx, cr); err != nil {
				t.Fatal(err)
			}
		}
		return observation.ResourceUpToDate
	}
	// Control: the unchanged Terraform Read falsely reports convergence after
	// reconstructing desired parameters, even with successful broker retrieval.
	if !observe(tfprovider.NewWithClient(srv.Client()), false) {
		t.Fatal("control must reproduce the original false convergence")
	}
	if observe(p, true) {
		t.Fatal("cache loss must not hide the unapplied broker parameter")
	}
	if updates != 1 || brokerParameters["policy"].(map[string]any)["enabled"] != true {
		t.Fatal("ordinary Update must heal broker drift")
	}
	if !observe(p, false) {
		t.Fatal("fresh connector must verify the applied update")
	}
	brokerParameters = map[string]any{"policy": map[string]any{"enabled": true, "broker_default": "retained"}, "count": 2, "extra": true}
	if !observe(p, false) {
		t.Fatal("fresh readback must converge when configured fields match, including broker defaults")
	}
	brokerParameters["count"] = 3
	if observe(p, false) {
		t.Fatal("external drift must be detected after another restart")
	}
	if reads < 4 {
		t.Fatalf("expected broker readback on each fresh connector, got %d", reads)
	}
	retrievable = false
	if !observe(p, false) {
		t.Fatal("non-retrievable offerings must preserve existing parameter behavior")
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

func readbackSetup(p fwprovider.Provider, url string) terraform.SetupFn {
	return func(context.Context, client.Client, xpresource.Managed) (terraform.Setup, error) {
		return terraform.Setup{FrameworkProvider: p, Configuration: map[string]any{
			"username": "test", "password": "test", "globalaccount": "example", "cli_server_url": url,
		}}, nil
	}
}
