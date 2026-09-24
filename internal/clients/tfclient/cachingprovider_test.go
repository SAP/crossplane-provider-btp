package tfclient

import (
	"context"
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
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"username": tftypes.String, "password": tftypes.String, "globalaccount": tftypes.String,
	}}
	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		"username":      tftypes.NewValue(tftypes.String, user),
		"password":      tftypes.NewValue(tftypes.String, "pw"),
		"globalaccount": tftypes.NewValue(tftypes.String, "ga"),
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

// blockingProvider blocks each Configure until released, so a test can hold a
// login open and prove other logins are not serialized behind it.
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

	// Wait until both logins are in flight. If different keys serialized, only
	// one Configure would ever start and this would hang (test timeout = failure).
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
