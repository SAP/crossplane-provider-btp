package tfclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tfprovider "github.com/SAP/terraform-provider-btp/btp/provider"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestWorkloadBothSetupPathsRereadToken(t *testing.T) {
	file := filepath.Join(t.TempDir(), "token")
	os.WriteFile(file, []byte("first-jwt"), 0600)
	pc := fakeProviderConfig(testProviderName, "manual-cis", testSecretNS, testGlobalAccount, testCliServerURL)
	pc.Spec.ServiceAccountSecret = v1alpha1.ProviderCredentials{}
	pc.Spec.WorkloadIdentity = &v1alpha1.WorkloadIdentityConfiguration{TokenFile: file, IdentityProvider: "test-origin", UserEmail: "workload@example.com"}
	kube := &test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
		if out, ok := obj.(*v1alpha1.ProviderConfig); ok {
			*out = *pc
			return nil
		}
		if _, ok := obj.(*corev1.Secret); ok {
			t.Fatal("workload setup read a Secret")
		}
		return nil
	}, MockList: test.NewMockListFn(nil)}
	mg := &fake.LegacyManaged{}
	mg.SetProviderConfigReference(&xpv1.Reference{Name: testProviderName})
	for _, builder := range []struct {
		name    string
		tracked bool
	}{{"tracked", true}, {"internal", false}} {
		t.Run(builder.name, func(t *testing.T) {
			setup := TerraformSetupBuilderNoTracking()
			if builder.tracked {
				setup = TerraformSetupBuilder()
			}
			for _, token := range []string{"first-jwt", "rotated-jwt"} {
				os.WriteFile(file, []byte(token), 0600)
				got, err := setup(context.Background(), kube, mg)
				if err != nil {
					t.Fatal(err)
				}
				if got.Configuration["assertion"] != token || got.Configuration["idp"] != "test-origin" {
					t.Fatal("incorrect assertion configuration")
				}
				for _, key := range []string{"username", "password"} {
					if _, exists := got.Configuration[key]; exists {
						t.Fatal("password input present")
					}
				}
			}
		})
	}
	pc.Spec.ServiceAccountSecret.Source = "Secret"
	if _, err := terraformConfiguration(context.Background(), kube, pc); err == nil {
		t.Fatal("accepted mixed user credentials")
	}
	pc.Spec.ServiceAccountSecret = v1alpha1.ProviderCredentials{}
	os.WriteFile(file, nil, 0600)
	if _, err := terraformConfiguration(context.Background(), kube, pc); err == nil {
		t.Fatal("accepted empty assertion")
	}
}

func TestRealTerraformAssertionLoginAndCacheRecovery(t *testing.T) {
	var assertions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["customIdp"] != "test-origin" || body["subdomain"] != "test-ga" {
			t.Error("incorrect login identity")
		}
		if body["password"] != nil && body["password"] != "" || body["userName"] != nil && body["userName"] != "" {
			t.Error("password login used")
		}
		assertions = append(assertions, body["jwt"].(string))
		w.Header().Set("X-Cpcli-Sessionid", "test-session")
		io.WriteString(w, `{"mail":"workload@example.com","issuer":"https://issuer.example"}`)
	}))
	defer server.Close()
	p := newCachingProvider(tfprovider.New())
	configure := func(token string) {
		ctx := context.Background()
		var schema fwprovider.SchemaResponse
		p.Schema(ctx, fwprovider.SchemaRequest{}, &schema)
		dv, err := protov5DynamicValueFromMap(map[string]any{"assertion": token, "idp": "test-origin", "globalaccount": "test-ga", "cli_server_url": server.URL}, schema.Schema.Type().TerraformType(ctx))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := providerserver.NewProtocol5(p)().ConfigureProvider(ctx, &tfprotov5.ConfigureProviderRequest{TerraformVersion: "crossTF000", Config: dv})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range resp.Diagnostics {
			if d.Severity == tfprotov5.DiagnosticSeverityError {
				t.Fatalf("assertion configure failed: %s", d.Summary)
			}
		}
	}
	configure(testAssertion("first"))
	configure(testAssertion("second"))
	if len(assertions) != 1 {
		t.Fatal("JWT rotation unnecessarily creates sessions")
	}
	p.evictBySubdomain("test-ga")
	configure(testAssertion("second"))
	if len(assertions) != 2 || assertions[1] != testAssertion("second") {
		t.Fatal("eviction did not use fresh assertion")
	}
	for _, entry := range p.entries {
		entry.created = time.Now().Add(-16 * time.Minute)
	}
	configure(testAssertion("third"))
	if len(assertions) != 3 || assertions[2] != testAssertion("third") {
		t.Fatal("bounded session cache did not reauthenticate")
	}
}

func testAssertion(generation string) string {
	payload := fmt.Sprintf(`{"iss":"https://issuer.example","sub":"system:serviceaccount:test:provider","jti":%q}`, generation)
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".signature"
}

func TestAssertionIdentitySeparatesPrincipals(t *testing.T) {
	if assertionIdentity(testAssertion("one")) != assertionIdentity(testAssertion("two")) {
		t.Fatal("rotation changed principal identity")
	}
	payload := `{"iss":"https://issuer.example","sub":"system:serviceaccount:test:other"}`
	other := "e30." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".signature"
	if assertionIdentity(other) == assertionIdentity(testAssertion("one")) {
		t.Fatal("different principals share cache")
	}
	if assertionIdentity("invalid-one") == assertionIdentity("invalid-two") {
		t.Fatal("malformed tokens share cache")
	}
}

type workloadResponseTransport struct{}

func (workloadResponseTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: http.Header{headerCLIBackendStatus: []string{"401"}}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: r}, nil
}
func TestWorkloadBackendUnauthorizedEvicts(t *testing.T) {
	evicted := ""
	transport := &cliTransport{base: workloadResponseTransport{}, evictSub: func(sd string) { evicted = sd }, evictAll: func() { t.Fatal("unexpected global eviction") }}
	req, _ := http.NewRequest("POST", "https://cli.example/command/v2.106.1/services/instance", nil)
	req.Header.Set(headerCLISessionId, "test-session")
	req.Header.Set(headerCLISubdomain, "test-ga")
	_, err := transport.RoundTrip(req)
	if err != nil || evicted != "test-ga" {
		t.Fatal("backend401 did not evict session")
	}
}
