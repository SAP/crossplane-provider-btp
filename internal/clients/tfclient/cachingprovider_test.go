package tfclient

import (
	"context"
	"errors"
	"fmt"
	"github.com/sap/crossplane-provider-btp/pkg/diagnostics"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

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

func TestCache(t *testing.T) {
	stub := &stubProvider{}
	p := newCachingProvider(stub)
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
	p := newCachingProvider(stub)
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
		t.Fatalf("concurrent same-key Configure must log in once, got %d", got)
	}
}

func TestDifferentKeysDoNotSerialize(t *testing.T) {
	stub := &blockingProvider{release: make(chan struct{})}
	p := newCachingProvider(stub)
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
	w := newCachingProvider(tfprovider.New())
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

func TestEvictBySubdomain(t *testing.T) {
	stub := &stubProvider{}
	p := newCachingProvider(stub)
	ctx := context.Background()

	var r provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{Config: cfgGA(t, "alice", "acct-A")}, &r)
	p.Configure(ctx, provider.ConfigureRequest{Config: cfgGA(t, "bob", "acct-B")}, &r)
	if len(p.entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(p.entries))
	}

	p.evictBySubdomain("acct-A")
	if len(p.entries) != 1 {
		t.Fatalf("evict acct-A must leave 1 entry, got %d", len(p.entries))
	}

	// Re-configuring A must miss the cache and log in again; B still cached.
	p.Configure(ctx, provider.ConfigureRequest{Config: cfgGA(t, "alice", "acct-A")}, &r)
	if got := stub.calls.Load(); got != 3 {
		t.Fatalf("want 3 logins (A, B, A-again), got %d", got)
	}
}

func TestEvictAll(t *testing.T) {
	stub := &stubProvider{}
	p := newCachingProvider(stub)
	ctx := context.Background()

	var r provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{Config: cfgGA(t, "alice", "acct-A")}, &r)
	p.Configure(ctx, provider.ConfigureRequest{Config: cfgGA(t, "bob", "acct-B")}, &r)

	p.evictAll()
	if len(p.entries) != 0 {
		t.Fatalf("evictAll must clear entries, got %d", len(p.entries))
	}

	p.Configure(ctx, provider.ConfigureRequest{Config: cfgGA(t, "alice", "acct-A")}, &r)
	if got := stub.calls.Load(); got != 3 {
		t.Fatalf("re-configure after evictAll must log in again, got %d logins", got)
	}
}

// rtFunc adapts a func to http.RoundTripper.
type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEvictTransportOn401(t *testing.T) {
	cases := []struct {
		name             string
		status           int
		sessionID        string
		subdomain        string
		wantSub, wantAll bool
	}{
		{"401 with sessionid+subdomain evicts that subdomain", 401, "sess", "acct-A", true, false},
		{"401 with sessionid, no subdomain evicts all", 401, "sess", "", false, true},
		{"401 without sessionid (login POST) evicts nothing", 401, "", "acct-A", false, false},
		{"200 evicts nothing", 200, "sess", "acct-A", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotSub string
			var subCalled, allCalled bool
			tr := &cliTransport{
				base: rtFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: tc.status, Body: http.NoBody}, nil
				}),
				evictSub: func(sd string) { subCalled = true; gotSub = sd },
				evictAll: func() { allCalled = true },
			}
			req, _ := http.NewRequest("POST", "https://cli.example/command", nil)
			if tc.sessionID != "" {
				req.Header.Set(headerCLISessionId, tc.sessionID)
			}
			if tc.subdomain != "" {
				req.Header.Set(headerCLISubdomain, tc.subdomain)
			}
			if _, err := tr.RoundTrip(req); err != nil {
				t.Fatal(err)
			}
			if subCalled != tc.wantSub || allCalled != tc.wantAll {
				t.Fatalf("evictSub=%v evictAll=%v, want %v/%v", subCalled, allCalled, tc.wantSub, tc.wantAll)
			}
			if tc.wantSub && gotSub != tc.subdomain {
				t.Fatalf("evictSub got %q, want %q", gotSub, tc.subdomain)
			}
		})
	}
}

// capLogger records the last Info and Debug call's message and key/values
// separately so tests can assert on both log levels.
type capLogger struct {
	msg      string
	kv       map[string]interface{}
	debugMsg string
	debugKV  map[string]interface{}
}

func (l *capLogger) Info(msg string, kv ...interface{}) {
	l.msg = msg
	l.kv = kvToMap(kv)
}
func (l *capLogger) Debug(msg string, kv ...interface{}) {
	l.debugMsg = msg
	l.debugKV = kvToMap(kv)
}
func (l *capLogger) WithValues(...interface{}) logging.Logger { return l }

func kvToMap(kv []interface{}) map[string]interface{} {
	m := map[string]interface{}{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[fmt.Sprint(kv[i])] = kv[i+1]
	}
	return m
}

func TestEvictTransportLogsFailures(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		backend    string // X-Cpcli-Backend-Status
		rtErr      error
		wantInfo   bool
		wantDebug  bool
		wantStatus int
		wantCorr   string
	}{
		{name: "200 proxy wrapping backend 429 logs the backend status", status: 200, backend: "429", wantInfo: true, wantStatus: 429, wantCorr: "corr-429"},
		{name: "transport 500 logs", status: 500, wantInfo: true, wantStatus: 500, wantCorr: "corr-123"},
		{name: "transport error logs", status: 0, rtErr: errors.New("dial fail"), wantInfo: true},
		{name: "200 with healthy backend logs at debug only", status: 200, backend: "200", wantDebug: true},
		{name: "plain 200 logs at debug only", status: 200, wantDebug: true},
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
				evictSub: func(string) {},
				evictAll: func() {},
				log:      log,
			}
			req, _ := http.NewRequest("POST", "https://cli.example/command", nil)
			req.Header.Set(headerCorrelationID, tc.wantCorr)
			tr.RoundTrip(req) //nolint:errcheck

			if tc.wantInfo != (log.msg != "") {
				t.Fatalf("Info logged=%v, want %v", log.msg != "", tc.wantInfo)
			}
			if tc.wantDebug != (log.debugMsg != "") {
				t.Fatalf("Debug logged=%v, want %v", log.debugMsg != "", tc.wantDebug)
			}
			if tc.wantInfo {
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
			}
			if tc.wantDebug {
				if got := log.debugMsg; got != "cli request ok" {
					t.Fatalf("debugMsg=%q, want %q", got, "cli request ok")
				}
				if got := log.debugKV["cliServerURL"]; got != "cli.example" {
					t.Fatalf("debug cliServerURL=%v, want cli.example", got)
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
		evictSub: func(string) {},
		evictAll: func() {},
		log:      log,
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

func TestCLITransportLogsResourceAndAttempt(t *testing.T) {
	log := &capLogger{}
	tr := &cliTransport{
		base: rtFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("timeout")
		}),
		evictSub: func(string) {},
		evictAll: func() {},
		log:      log,
	}
	ctx := diagnostics.BeginRequest(WithReconcileTrace(context.Background(), "ServiceManager/sm-01a0c329"))
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://cli.example/command", nil)
	req.Header.Set(headerCorrelationID, "corr-xyz")

	tr.RoundTrip(req) //nolint:errcheck // first attempt
	if got := log.kv["resource"]; got != "ServiceManager/sm-01a0c329" {
		t.Fatalf("resource=%v, want ServiceManager/sm-01a0c329", got)
	}
	if got := log.kv["attempt"]; got != 1 {
		t.Fatalf("attempt=%v, want 1", got)
	}

	tr.RoundTrip(req) //nolint:errcheck // second attempt (same correlationID = retry)
	if got := log.kv["attempt"]; got != 2 {
		t.Fatalf("attempt=%v on retry, want 2", got)
	}
}
