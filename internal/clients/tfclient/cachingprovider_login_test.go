package tfclient

// Login-count test over the real tfprovider.New() wrapped in newCachingProvider,
// as opposed to the stubProvider in cachingprovider_test.go. We cannot call the
// login directly, so we run the provider like upjet does (protocol-v5 server +
// ConfigureProvider) and count the logins that reach a fake btp-CLI server.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	tfprovider "github.com/SAP/terraform-provider-btp/btp/provider"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// protov5DynamicValueFromMap mirrors upjet's helper of the same name
// (pkg/controller/external_tfpluginfw.go).
func protov5DynamicValueFromMap(data map[string]any, terraformType tftypes.Type) (*tfprotov5.DynamicValue, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}
	tfValue, err := tftypes.ValueFromJSONWithOpts(jsonBytes, terraformType, tftypes.ValueFromJSONOpts{IgnoreUndefinedAttributes: true})
	if err != nil {
		return nil, fmt.Errorf("tf value from json: %w", err)
	}
	dv, err := tfprotov5.NewDynamicValue(terraformType, tfValue)
	if err != nil {
		return nil, fmt.Errorf("dynamic value: %w", err)
	}
	return &dv, nil
}

// noForkConfigureOnce replicates upjet's configureProvider: fresh protocol-v5
// server around the provider, ConfigureProvider with the ROPC config.
func noForkConfigureOnce(ctx context.Context, p fwprovider.Provider, cliServerURL, password string) error {
	return noForkConfigureAs(ctx, p, cliServerURL, "technical-user", password)
}

func noForkConfigureAs(ctx context.Context, p fwprovider.Provider, cliServerURL, username, password string) error {
	var sr fwprovider.SchemaResponse
	p.Schema(ctx, fwprovider.SchemaRequest{}, &sr)
	if sr.Diagnostics.HasError() {
		return fmt.Errorf("provider schema: %v", sr.Diagnostics)
	}
	srv := providerserver.NewProtocol5(p)()
	dv, err := protov5DynamicValueFromMap(map[string]any{
		"username":       username,
		"password":       password,
		"globalaccount":  "ga-subdomain",
		"cli_server_url": cliServerURL,
	}, sr.Schema.Type().TerraformType(ctx))
	if err != nil {
		return err
	}
	resp, err := srv.ConfigureProvider(ctx, &tfprotov5.ConfigureProviderRequest{TerraformVersion: "crossTF000", Config: dv})
	if err != nil {
		return err
	}
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov5.DiagnosticSeverityError {
			return fmt.Errorf("configure diagnostic: %s: %s", d.Summary, d.Detail)
		}
	}
	return nil
}

// fakeBTPCLI models the btp-CLI login endpoint, counting login POSTs.
type fakeBTPCLI struct {
	password string
	logins   atomic.Int64
}

func (f *fakeBTPCLI) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) < len("/login") || r.URL.Path[:len("/login")] != "/login" {
			http.NotFound(w, r)
			return
		}
		f.logins.Add(1)
		var body struct {
			Password string `json:"password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Password != f.password {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad credentials"})
			return
		}
		w.Header().Set("X-Cpcli-Sessionid", "sess")
		_ = json.NewEncoder(w).Encode(map[string]string{"mail": "u@example.com", "issuer": "https://idp.example"})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// N concurrent reconciles through the caching wrapper hit the login endpoint
// once; the rest replay the cached session.
func TestCachingWrapper_ConcurrentReconciles_OneLogin(t *testing.T) {
	t.Parallel()
	const N = 100
	fake := &fakeBTPCLI{password: "correct-pw"}
	srv := fake.start(t)
	p := newCachingProvider(tfprovider.New())
	ctx := context.Background()

	var wg sync.WaitGroup
	var failed atomic.Int64
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := noForkConfigureOnce(ctx, p, srv.URL, "correct-pw"); err != nil {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := failed.Load(); got != 0 {
		t.Errorf("failed configures = %d, want 0 (correct password)", got)
	}
	if got := fake.logins.Load(); got != 1 {
		t.Errorf("btp-CLI logins = %d, want 1 (cache collapses %d reconciles)", got, N)
	}
}

// Two credential sets are cached separately: each logs in once, so repeated
// reconciles for both users produce exactly two logins.
func TestCachingWrapper_TwoCredentials_TwoLogins(t *testing.T) {
	t.Parallel()
	const N = 50
	fake := &fakeBTPCLI{password: "correct-pw"}
	srv := fake.start(t)
	p := newCachingProvider(tfprovider.New())
	ctx := context.Background()

	var wg sync.WaitGroup
	var failed atomic.Int64
	for i := 0; i < N; i++ {
		wg.Add(1)
		user := "user-a"
		if i%2 == 1 {
			user = "user-b"
		}
		go func() {
			defer wg.Done()
			if err := noForkConfigureAs(ctx, p, srv.URL, user, "correct-pw"); err != nil {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := failed.Load(); got != 0 {
		t.Errorf("failed configures = %d, want 0 (correct password)", got)
	}
	if got := fake.logins.Load(); got != 2 {
		t.Errorf("btp-CLI logins = %d, want 2 (one per credential set)", got)
	}
}
