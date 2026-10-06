package tfclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

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

// fakeBTPCLI models the btp-CLI login endpoint with an optional per-user lockout.
// lockThreshold == 0 means no lockout (simple counting mode).
type fakeBTPCLI struct {
	password      string
	lockThreshold int // 0 = no lockout

	mu            sync.Mutex
	failedLogins  int
	maxConsecFail int
	locked        bool

	logins atomic.Int64 // total POSTs to /login
	ok     atomic.Int64
	bad    atomic.Int64
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

		f.mu.Lock()
		defer f.mu.Unlock()
		if f.locked {
			f.bad.Add(1)
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
			if f.lockThreshold > 0 && f.failedLogins >= f.lockThreshold {
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
