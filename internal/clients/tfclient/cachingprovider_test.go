package tfclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	fwschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	tfprovider "github.com/SAP/terraform-provider-btp/btp/provider"
)

// stubProvider counts Configure calls and hands back a sentinel client.
type stubProvider struct {
	provider.Provider
	calls atomic.Int64
}

func (s *stubProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = fwschema.Schema{Attributes: map[string]fwschema.Attribute{
		"username":      fwschema.StringAttribute{Optional: true},
		"password":      fwschema.StringAttribute{Optional: true},
		"globalaccount": fwschema.StringAttribute{Optional: true},
	}}
}
func (s *stubProvider) Configure(_ context.Context, _ provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	n := s.calls.Add(1)
	resp.ResourceData = &struct{ id int64 }{id: n}
}

func cfg(t *testing.T, user string) tfsdk.Config {
	return cfgGA(t, user, "ga")
}

func cfgGA(t *testing.T, user, ga string) tfsdk.Config {
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"username": tftypes.String, "password": tftypes.String, "globalaccount": tftypes.String,
	}}
	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		"username":      tftypes.NewValue(tftypes.String, user),
		"password":      tftypes.NewValue(tftypes.String, "pw"),
		"globalaccount": tftypes.NewValue(tftypes.String, ga),
	})
	return tfsdk.Config{Raw: raw, Schema: rschema.Schema{Attributes: map[string]rschema.Attribute{
		"username":      rschema.StringAttribute{Optional: true},
		"password":      rschema.StringAttribute{Optional: true},
		"globalaccount": rschema.StringAttribute{Optional: true},
	}}}
}

func key(t *testing.T, user, ga string) string {
	return fmt.Sprintf("%v", cfgGA(t, user, ga).Raw)
}

// TestCache: a single-session pool (size 1) logs in once per key and replays the
// same client, the old reuse=true behaviour.
func TestCache(t *testing.T) {
	stub := &stubProvider{}
	p := newTerraformProvider(stub, poolConfig{size: 1})
	ctx := context.Background()

	var r1 provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "alice")}, &r1)
	var r2 provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "alice")}, &r2)
	if stub.calls.Load() != 1 {
		t.Fatalf("same creds must configure once, got %d", stub.calls.Load())
	}
	if r1.ResourceData != r2.ResourceData {
		t.Fatal("cache hit must replay the same client")
	}

	var r3 provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "bob")}, &r3)
	if stub.calls.Load() != 2 {
		t.Fatalf("different creds must reconfigure, got %d", stub.calls.Load())
	}
}

// TestPoolDisabled: size 0 bypasses the pool, logging in on every reconcile.
func TestPoolDisabled(t *testing.T) {
	stub := &stubProvider{}
	p := newTerraformProvider(stub, poolConfig{size: 0})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		var r provider.ConfigureResponse
		p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "alice")}, &r)
	}
	if got := stub.calls.Load(); got != 3 {
		t.Fatalf("size 0 must configure per reconcile, got %d", got)
	}
}

// TestRoundRobinCapsLogins: an N-slot pool logs in at most N times for one key,
// however many reconciles hit it.
func TestRoundRobinCapsLogins(t *testing.T) {
	stub := &stubProvider{}
	const N = 3
	p := newTerraformProvider(stub, poolConfig{size: N})
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		var r provider.ConfigureResponse
		p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "alice")}, &r)
	}
	if got := stub.calls.Load(); got != N {
		t.Fatalf("pool of %d must cap logins at %d, got %d", N, N, got)
	}
}

// TestDirtyRewarms: a slot flagged dirty re-authenticates on the next checkout.
func TestDirtyRewarms(t *testing.T) {
	stub := &stubProvider{}
	p := newTerraformProvider(stub, poolConfig{size: 1})
	ctx := context.Background()

	var r provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "alice")}, &r)
	if stub.calls.Load() != 1 {
		t.Fatalf("first configure must log in, got %d", stub.calls.Load())
	}

	p.pools[key(t, "alice", "ga")].slots[0].dirty.Store(true)

	p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "alice")}, &r)
	if got := stub.calls.Load(); got != 2 {
		t.Fatalf("dirty slot must re-login, got %d", got)
	}
}

// TestTTLRewarms: a slot past its TTL re-authenticates on the next checkout.
func TestTTLRewarms(t *testing.T) {
	stub := &stubProvider{}
	var now atomic.Int64
	now.Store(time.Now().UnixNano())
	p := newTerraformProvider(stub, poolConfig{size: 1, ttl: time.Minute, now: func() time.Time {
		return time.Unix(0, now.Load())
	}})
	ctx := context.Background()

	var r provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "alice")}, &r)
	p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "alice")}, &r)
	if stub.calls.Load() != 1 {
		t.Fatalf("within TTL must reuse, got %d logins", stub.calls.Load())
	}

	now.Add(int64(2 * time.Minute))
	p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "alice")}, &r)
	if got := stub.calls.Load(); got != 2 {
		t.Fatalf("past TTL must re-login, got %d", got)
	}
}

// TestMarkDirtyBySession: a 401's session id marks exactly the owning slot.
func TestMarkDirtyBySession(t *testing.T) {
	p := newTerraformProvider(&stubProvider{}, poolConfig{size: 2})
	s := &sessionSlot{subdomain: "acct-A"}
	p.sessions["sess-1"] = s

	p.markDirtyBySession("sess-1")
	if !s.dirty.Load() {
		t.Fatal("markDirtyBySession must flag the slot owning the session id")
	}
	p.markDirtyBySession("unknown") // must be a no-op, not panic
}

// blockingProvider blocks each Configure until released.
type blockingProvider struct {
	provider.Provider
	calls   atomic.Int64
	release chan struct{}
}

func (s *blockingProvider) Configure(_ context.Context, _ provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	s.calls.Add(1)
	<-s.release
	resp.ResourceData = &struct{}{}
}

func TestConcurrentSameKeyLogsInOnce(t *testing.T) {
	stub := &stubProvider{}
	p := newTerraformProvider(stub, poolConfig{size: 1})
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var r provider.ConfigureResponse
			p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "alice")}, &r)
		}()
	}
	wg.Wait()

	if got := stub.calls.Load(); got != 1 {
		t.Fatalf("concurrent same-key Configure on a size-1 pool must log in once, got %d", got)
	}
}

func TestDifferentKeysDoNotSerialize(t *testing.T) {
	stub := &blockingProvider{release: make(chan struct{})}
	p := newTerraformProvider(stub, poolConfig{size: 1})
	ctx := context.Background()

	// Hold alice's login open; bob must still reach its own Configure.
	go func() {
		var r provider.ConfigureResponse
		p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "alice")}, &r)
	}()

	done := make(chan struct{})
	go func() {
		var r provider.ConfigureResponse
		p.Configure(ctx, provider.ConfigureRequest{Config: cfg(t, "bob")}, &r)
		close(done)
	}()

	// Wait until both logins are in flight; if different keys serialized, only
	// one would ever start and this hangs (test timeout = failure).
	for stub.calls.Load() != 2 {
		runtime.Gosched()
	}
	close(stub.release)
	<-done
	if got := stub.calls.Load(); got != 2 {
		t.Fatalf("both keys must configure, got %d", got)
	}
}

func TestOptionalInterfacePreserved(t *testing.T) {
	w := newTerraformProvider(tfprovider.New(), poolConfig{size: 1})
	if _, ok := interface{}(w).(provider.ProviderWithFunctions); !ok {
		t.Fatal("wrapper lost ProviderWithFunctions")
	}
	if _, ok := interface{}(w).(provider.ProviderWithListResources); !ok {
		t.Fatal("wrapper lost ProviderWithListResources")
	}
	if _, ok := interface{}(w).(provider.ProviderWithActions); !ok {
		t.Fatal("wrapper lost ProviderWithActions")
	}
}

// rtFunc adapts a func to http.RoundTripper.
type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTransportMarksDirtyOn401(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		sessionID string
		wantDirty string
	}{
		{"401 with sessionid marks that session dirty", 401, "sess", "sess"},
		{"401 without sessionid (login POST) marks nothing", 401, "", ""},
		{"200 marks nothing", 200, "sess", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var dirtied string
			var captured bool
			tr := &cliTransport{
				base: rtFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: tc.status, Body: http.NoBody}, nil
				}),
				capture:   func(context.Context, *http.Response) { captured = true },
				markDirty: func(id string) { dirtied = id },
			}
			req, _ := http.NewRequest("POST", "https://cli.example/command", nil)
			if tc.sessionID != "" {
				req.Header.Set(headerCLISessionId, tc.sessionID)
			}
			if _, err := tr.RoundTrip(req); err != nil {
				t.Fatal(err)
			}
			if dirtied != tc.wantDirty {
				t.Fatalf("markDirty got %q, want %q", dirtied, tc.wantDirty)
			}
			if !captured {
				t.Fatal("capture must run on every round trip")
			}
		})
	}
}

// capLogger records the last Info call's message and key/values.
type capLogger struct {
	msg string
	kv  map[string]interface{}
}

func (l *capLogger) Info(msg string, kv ...interface{}) {
	l.msg = msg
	l.kv = map[string]interface{}{}
	for i := 0; i+1 < len(kv); i += 2 {
		l.kv[fmt.Sprint(kv[i])] = kv[i+1]
	}
}
func (l *capLogger) Debug(string, ...interface{})             {}
func (l *capLogger) WithValues(...interface{}) logging.Logger { return l }

func TestEvictTransportLogsFailures(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		backend    string // X-Cpcli-Backend-Status
		rtErr      error
		wantLog    bool
		wantStatus int
		wantCorr   string
	}{
		{name: "200 proxy wrapping backend 429 logs the backend status", status: 200, backend: "429", wantLog: true, wantStatus: 429, wantCorr: "corr-429"},
		{name: "transport 500 logs", status: 500, wantLog: true, wantStatus: 500, wantCorr: "corr-123"},
		{name: "transport error logs", status: 0, rtErr: errors.New("dial fail"), wantLog: true},
		{name: "200 with healthy backend stays silent", status: 200, backend: "200", wantLog: false},
		{name: "plain 200 stays silent", status: 200, wantLog: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := &capLogger{}
			tr := &cliTransport{
				base: rtFunc(func(*http.Request) (*http.Response, error) {
					if tc.rtErr != nil {
						return nil, tc.rtErr
					}
					h := http.Header{}
					h.Set(headerCorrelationID, tc.wantCorr)
					if tc.backend != "" {
						h.Set(headerCLIBackendStatus, tc.backend)
					}
					return &http.Response{StatusCode: tc.status, Header: h, Body: http.NoBody}, nil
				}),
				log: log,
			}
			req, _ := http.NewRequest("POST", "https://cli.example/command", nil)
			tr.RoundTrip(req) //nolint:errcheck

			if tc.wantLog != (log.msg != "") {
				t.Fatalf("logged=%v, want %v", log.msg != "", tc.wantLog)
			}
			if !tc.wantLog {
				return
			}
			if got := log.kv["cliServerURL"]; got != "cli.example" {
				t.Fatalf("cliServerURL=%v, want cli.example", got)
			}
			if tc.rtErr == nil {
				if got := log.kv["status"]; got != tc.wantStatus {
					t.Fatalf("status=%v, want %d", got, tc.wantStatus)
				}
				if got := log.kv["correlationID"]; got != tc.wantCorr {
					t.Fatalf("correlationID=%v, want %q", got, tc.wantCorr)
				}
			}
		})
	}
}

// The CLI server's 500 body carries the real failure reason (#799). Assert we log
// it AND leave the body fully readable for the downstream consumer.
func TestCLITransportPeeksBodyWithoutConsumingIt(t *testing.T) {
	const msg = "The CLI server is currently experiencing difficulties connecting to the BTP"
	log := &capLogger{}
	tr := &cliTransport{
		base: rtFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 500,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(msg)),
			}, nil
		}),
		log: log,
	}
	req, _ := http.NewRequest("POST", "https://cli.example/command", nil)
	resp, _ := tr.RoundTrip(req)

	if got := log.kv["body"]; got != msg {
		t.Fatalf("logged body=%q, want %q", got, msg)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != msg {
		t.Fatalf("downstream body=%q, want full %q", got, msg)
	}
}
