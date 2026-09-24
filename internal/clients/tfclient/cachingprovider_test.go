package tfclient

import (
	"context"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

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
			tr := &evictTransport{
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
