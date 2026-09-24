package tfclient

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/function"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
)

// cachingProvider wraps the BTP plugin-framework provider and caches the result
// of Configure keyed on the resolved provider configuration.
//
// upjet's no-fork framework connector rebuilds a provider server and fires a
// ConfigureProvider RPC on every reconcile (upjet
// pkg/controller/external_tfpluginfw.go configureProvider). The BTP provider's
// Configure logs in unconditionally, so that RPC is one BTP login per
// reconcile — the ~1:1 login/request ratio tracked in #702.
//
// Because frameworkProvider() is a process-wide singleton (sync.OnceValue), the
// same wrapper instance receives every ConfigureProvider RPC. On a cache hit we
// replay the already-logged-in client onto resp instead of delegating to the
// inner Configure, so no new login happens. Configure is the only channel from
// the provider to its resources (resp.*Data reaches each resource's Configure),
// so replaying those fields is equivalent to reconfiguring.
//
// ponytail: no TTL / no auth-error invalidation. The BTP session id has no
// client-visible expiry and the server can revoke it; a stale entry surfaces as
// a 401/403 on a data request, not here. Add TTL + evict-on-auth-error if the
// PoC measurements show revocation churn.
type cachingProvider struct {
	fwprovider.Provider // forwards Metadata/Schema/Resources/DataSources

	mu      sync.Mutex
	entries map[string]*cacheEntry
}

// cacheEntry logs in at most once per key. once.Do serializes concurrent
// reconciles for the same credentials onto a single login; different keys hold
// different entries and log in in parallel.
type cacheEntry struct {
	once sync.Once
	resp *fwprovider.ConfigureResponse // nil if the login errored (login retried next reconcile)
}

func newCachingProvider(inner fwprovider.Provider) *cachingProvider {
	return &cachingProvider{
		Provider: inner,
		entries:  map[string]*cacheEntry{},
	}
}

func (p *cachingProvider) Configure(ctx context.Context, req fwprovider.ConfigureRequest, resp *fwprovider.ConfigureResponse) {
	// Key on the whole raw provider config. Schema-agnostic: it can't drift when
	// terraform-provider-btp adds an attribute, and it already covers every field
	// a login flow branches on (credentials, endpoint, idp, certs).
	key := fmt.Sprintf("%v", req.Config.Raw)

	// Global lock only guards the map lookup, not the login. A failed login
	// clears the entry so the next reconcile retries instead of caching an error.
	p.mu.Lock()
	e, ok := p.entries[key]
	if !ok {
		e = &cacheEntry{}
		p.entries[key] = e
	}
	p.mu.Unlock()

	e.once.Do(func() {
		p.Provider.Configure(ctx, req, resp)
		if resp.Diagnostics.HasError() {
			p.mu.Lock()
			delete(p.entries, key)
			p.mu.Unlock()
			return
		}
		// Copy without Diagnostics so a later hit doesn't replay stale warnings.
		e.resp = &fwprovider.ConfigureResponse{
			ResourceData:          resp.ResourceData,
			DataSourceData:        resp.DataSourceData,
			ListResourceData:      resp.ListResourceData,
			ActionData:            resp.ActionData,
			EphemeralResourceData: resp.EphemeralResourceData,
		}
	})

	if e.resp != nil {
		resp.ResourceData = e.resp.ResourceData
		resp.DataSourceData = e.resp.DataSourceData
		resp.ListResourceData = e.resp.ListResourceData
		resp.ActionData = e.resp.ActionData
		resp.EphemeralResourceData = e.resp.EphemeralResourceData
	}
}

// Functions forwards to the inner provider. Embedding fwprovider.Provider does
// not promote it (the base interface omits Functions), so without this the
// wrapper silently fails the providerserver ProviderWithFunctions assertion and
// the btp_* provider functions disappear.
func (p *cachingProvider) Functions(ctx context.Context) []func() function.Function {
	if wf, ok := p.Provider.(fwprovider.ProviderWithFunctions); ok {
		return wf.Functions(ctx)
	}
	return nil
}
