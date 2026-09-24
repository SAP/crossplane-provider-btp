package tfclient

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// btpcli sets these on every data request once a session exists; eviction reads
// them off a 401'd request. Hard-coded because btpcli is internal/ and unimportable.
const (
	headerCLISessionId = "X-Cpcli-Sessionid"
	headerCLISubdomain = "X-Cpcli-Subdomain"
)

// cachingProvider caches Configure so upjet's per-reconcile ConfigureProvider
// RPC does not log in to BTP every time (the ~1:1 login/request ratio in #702).
// On a cache hit it replays the logged-in client onto resp instead of calling
// the inner Configure. A 401 to a data request evicts by globalaccount subdomain.
type cachingProvider struct {
	fwprovider.Provider // forwards Metadata/Schema/Resources/DataSources

	mu      sync.Mutex
	entries map[string]*cacheEntry
}

// cacheEntry logs in at most once per key via once.Do; different keys log in in parallel.
type cacheEntry struct {
	once      sync.Once
	resp      *fwprovider.ConfigureResponse
	subdomain string                        // globalaccount from config; eviction index
}

func newCachingProvider(inner fwprovider.Provider) *cachingProvider {
	return &cachingProvider{
		Provider: inner,
		entries:  map[string]*cacheEntry{},
	}
}

func (p *cachingProvider) Configure(ctx context.Context, req fwprovider.ConfigureRequest, resp *fwprovider.ConfigureResponse) {
	// Raw config as key: schema-agnostic, so it can't drift when the provider
	// adds an attribute, and it covers every field a login branches on.
	key := fmt.Sprintf("%v", req.Config.Raw)

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
			// Drop the entry so the next reconcile retries instead of caching an error.
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
		var sd types.String
		req.Config.GetAttribute(ctx, path.Root("globalaccount"), &sd)
		e.subdomain = sd.ValueString()
	})

	if e.resp != nil {
		resp.ResourceData = e.resp.ResourceData
		resp.DataSourceData = e.resp.DataSourceData
		resp.ListResourceData = e.resp.ListResourceData
		resp.ActionData = e.resp.ActionData
		resp.EphemeralResourceData = e.resp.EphemeralResourceData
	}
}

// Functions forwards to the inner provider
func (p *cachingProvider) Functions(ctx context.Context) []func() function.Function {
	if wf, ok := p.Provider.(fwprovider.ProviderWithFunctions); ok {
		return wf.Functions(ctx)
	}
	return nil
}

// evictBySubdomain drops every entry matching subdomain so the next reconcile re-logs-in.
func (p *cachingProvider) evictBySubdomain(subdomain string) {
	if subdomain == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, e := range p.entries {
		if e.subdomain == subdomain {
			delete(p.entries, k)
		}
	}
}

// evictAll clears the cache when a 401 has a session id but no subdomain header;
// used as fallback if subdomain header is not provided
func (p *cachingProvider) evictAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries = map[string]*cacheEntry{}
}

// evictTransport evicts the cached session on a 401 to a data request, so a
// transient 401 can't get stuck in the cache as a permanent one.
type evictTransport struct {
	base     http.RoundTripper
	evictSub func(subdomain string)
	evictAll func()
}

func (t *evictTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(r)
	// The session-id check skips login POSTs (no session headers), so a bad-creds
	// login 401 can't evict a valid entry.
	if resp != nil && resp.StatusCode == http.StatusUnauthorized && r.Header.Get(headerCLISessionId) != "" {
		if sd := r.Header.Get(headerCLISubdomain); sd != "" {
			t.evictSub(sd)
		} else {
			t.evictAll()
		}
	}
	return resp, err
}
