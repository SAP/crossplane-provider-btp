package tfclient

// Model-based concurrency test for the no-fork framework auth path (path 3 of 3
// in docs/development/concurrency-and-lockout-analysis.md).
//
// upjet's TerraformPluginFrameworkConnector.Connect calls configureProvider on
// EVERY reconcile with no cache guard: it wraps the (singleton) framework provider
// in a fresh protocol-v5 server and calls ConfigureProvider, which for the
// userPasswordFlow makes terraform-provider-btp POST a ROPC login to the btp-CLI
// server (/login/<ver>). The auth token is NOT reused across reconciles.
//
// This test replicates that exact configureProvider dance over the REAL
// tfprovider.New() against a fake btp-CLI server (login endpoint + 5-strike
// lockout model), proving: N concurrent reconciles → N logins (no reuse), and a
// wrong credential locks the shared technical user. btpcli is an internal package
// so it cannot be called directly; driving the provider's public Configure is the
// faithful seam.

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
// (pkg/controller/external_tfpluginfw.go): JSON-encode the config map, build a
// tftypes value against the provider schema (ignoring unset attributes), wrap as
// a protocol-v5 DynamicValue.
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
// server around the shared provider, ConfigureProvider with the ROPC config →
// one login. Returns an error if the provider reported configure diagnostics
// (e.g. bad credentials / locked).
func noForkConfigureOnce(ctx context.Context, p fwprovider.Provider, cliServerURL, password string) error {
	var sr fwprovider.SchemaResponse
	p.Schema(ctx, fwprovider.SchemaRequest{}, &sr)
	if sr.Diagnostics.HasError() {
		return fmt.Errorf("provider schema: %v", sr.Diagnostics)
	}
	srv := providerserver.NewProtocol5(p)()
	dv, err := protov5DynamicValueFromMap(map[string]any{
		"username":       "technical-user",
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

// fakeBTPCLI models the btp-CLI login endpoint with a per-user 5-strike lockout.
type fakeBTPCLI struct {
	password      string
	lockThreshold int

	mu            sync.Mutex
	failedLogins  int
	maxConsecFail int
	locked        bool

	logins atomic.Int64 // POSTs to /login
	ok     atomic.Int64
	bad    atomic.Int64
}

func (f *fakeBTPCLI) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// btp-CLI login path is /login/<version>[/...]; match the prefix.
		if len(r.URL.Path) < len("/login") || r.URL.Path[:len("/login")] != "/login" {
			http.NotFound(w, r)
			return
		}
		f.logins.Add(1)
		var body struct {
			Password string `json:"password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		f.mu.Lock()
		defer f.mu.Unlock()
		if f.locked {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "user locked"})
			return
		}
		if body.Password != f.password {
			f.failedLogins++
			if f.failedLogins > f.maxConsecFail {
				f.maxConsecFail = f.failedLogins
			}
			f.bad.Add(1)
			if f.failedLogins >= f.lockThreshold {
				f.locked = true
			}
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad credentials"})
			return
		}
		f.failedLogins = 0
		f.ok.Add(1)
		w.Header().Set("X-Cpcli-Sessionid", "sess")
		_ = json.NewEncoder(w).Encode(map[string]string{"mail": "u@example.com", "issuer": "https://idp.example"})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// 100 concurrent no-fork reconciles with the CORRECT password: each Connect
// re-configures the provider → a fresh login, so 100 logins hit the btp-CLI
// server (no reuse across reconciles), and the user is never locked.
func TestNoFork_100ConcurrentCorrectLogins_NeverLock(t *testing.T) {
	t.Parallel()
	const N = 100
	fake := &fakeBTPCLI{password: "correct-pw", lockThreshold: 5}
	srv := fake.start(t)
	p := tfprovider.New() // singleton, as tfclient.frameworkProvider() is
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
	if got := fake.ok.Load(); got != N {
		t.Errorf("btp-CLI logins = %d, want %d (one per reconcile, no reuse)", got, N)
	}
	if got := fake.logins.Load(); got != N {
		t.Errorf("total login POSTs = %d, want %d", got, N)
	}
	fake.mu.Lock()
	locked := fake.locked
	fake.mu.Unlock()
	if locked {
		t.Error("100 concurrent CORRECT no-fork logins must never lock the user")
	}
}

// Wrong password under concurrent no-fork reconciles locks the technical user.
func TestNoFork_WrongPassword_LocksAfterThreshold(t *testing.T) {
	t.Parallel()
	const N = 32
	fake := &fakeBTPCLI{password: "correct-pw", lockThreshold: 5}
	srv := fake.start(t)
	p := tfprovider.New()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = noForkConfigureOnce(ctx, p, srv.URL, "WRONG") }()
	}
	wg.Wait()

	fake.mu.Lock()
	locked, maxConsec := fake.locked, fake.maxConsecFail
	fake.mu.Unlock()
	if !locked {
		t.Error("wrong password under concurrent no-fork reconciles must lock the user")
	}
	if maxConsec < fake.lockThreshold {
		t.Errorf("max consecutive failures = %d, want ≥ threshold %d", maxConsec, fake.lockThreshold)
	}
}
