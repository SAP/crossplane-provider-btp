package tfclient

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/sap/crossplane-provider-btp/pkg/diagnostics"
)

// btpcli sets these on every data request once a session exists; eviction reads
// them off a 401'd request. Hard-coded because btpcli is internal/ and unimportable.
const (
	headerCLISessionId     = "X-Cpcli-Sessionid"
	headerCLISubdomain     = "X-Cpcli-Subdomain"
	headerCorrelationID    = "X-Correlationid"
	headerCLIBackendStatus = "X-Cpcli-Backend-Status"
)

// WithReconcileTrace is retained for callers that supply a resource name directly.
func WithReconcileTrace(ctx context.Context, trace string) context.Context {
	return diagnostics.WithResource(ctx, trace)
}

func reconcileTraceFrom(ctx context.Context) string { return diagnostics.Resource(ctx) }

// cachingProvider caches Configure so upjet's per-reconcile ConfigureProvider RPC does not
// log in to BTP on every reconcile (#702). Evicts on 401 by subdomain so a session expiry
// doesn't get stuck.
type cachingProvider struct {
	fwprovider.Provider // forwards Metadata/Schema/Resources/DataSources

	mu      sync.Mutex
	entries map[string]*cacheEntry
}

// cacheEntry: once.Do collapses concurrent reconciles for the same key to one login.
type cacheEntry struct {
	once      sync.Once
	resp      *fwprovider.ConfigureResponse
	subdomain string // globalaccount from config; eviction index
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

// ListResources forwards to the inner provider
func (p *cachingProvider) ListResources(ctx context.Context) []func() list.ListResource {
	if pl, ok := p.Provider.(fwprovider.ProviderWithListResources); ok {
		return pl.ListResources(ctx)
	}
	return nil
}

// Actions forwards to the inner provider
func (p *cachingProvider) Actions(ctx context.Context) []func() action.Action {
	if pa, ok := p.Provider.(fwprovider.ProviderWithActions); ok {
		return pa.Actions(ctx)
	}
	return nil
}

// evictBySubdomain drops cached sessions for a subdomain so the next reconcile re-authenticates.
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

// evictAll is the fallback when a 401 carries a session ID but no subdomain header.
func (p *cachingProvider) evictAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries = map[string]*cacheEntry{}
}

// cliTransport is the single injection point for all CLI/TF requests. It evicts the session on
// 401 (so a transient auth failure can't permanently poison the cache) and logs failures with
// enough context to diagnose a CLI-server outage
type cliTransport struct {
	base     http.RoundTripper
	evictSub func(subdomain string)
	evictAll func()
	log      logging.Logger
}

func (t *cliTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	attempt := diagnostics.NextAttempt(r.Context())
	start := time.Now()
	timing := &requestTiming{start: start, connectionMs: -1, tlsMs: -1, firstByteMs: -1}
	r = r.WithContext(httptrace.WithClientTrace(r.Context(), timing.trace()))
	resp, err := t.base.RoundTrip(r)
	diagnostics.Record(r.Context(), "btp HTTP attempt finished", err, append(timing.fields(), "durationMs", time.Since(start).Milliseconds(), "correlationID", r.Header.Get(headerCorrelationID), "attempt", attempt, "path", r.URL.Path, "action", r.URL.RawQuery)...)
	if resp != nil && resp.Body != nil {
		resp.Body = &observedBody{ReadCloser: resp.Body, ctx: r.Context(), start: time.Now(), request: r}
	}
	t.logResult(r, resp, err, time.Since(start), reconcileTraceFrom(r.Context()), attempt)
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

// logResult logs every CLI call: failures at Info (always visible), successes at
// Debug (visible only with --debug). The CLI proxy almost always returns HTTP 200
// — the real status is in X-Cpcli-Backend-Status; both are checked. The body on
// failures carries the CLI server's human-readable error, which upjet's newTFError
// swallows without logging it — we capture it here so the full error reason is
// visible in provider logs.
func (t *cliTransport) logResult(r *http.Request, resp *http.Response, err error, d time.Duration, trace string, attempt int) {
	if t.log == nil {
		return
	}
	// Backend status wins when present: it's the real result behind the 200 proxy.
	status := 0
	if resp != nil {
		status = resp.StatusCode
		if b, e := strconv.Atoi(resp.Header.Get(headerCLIBackendStatus)); e == nil {
			status = b
		}
	}

	kv := []interface{}{
		"method", r.Method,
		"path", r.URL.Path,
		"cliServerURL", r.URL.Host,
		"durationMs", d.Milliseconds(),
		"action", r.URL.RawQuery,
		"correlationID", r.Header.Get(headerCorrelationID),
		"reconcileID", diagnostics.ReconcileID(r.Context()),
		"stage", diagnostics.StageName(r.Context()),
		"remainingMs", diagnostics.Remaining(r.Context()),
		"contextError", diagnostics.ContextError(r.Context()),
		"resource", trace,
		"session", sessionFingerprint(r.Header.Get(headerCLISessionId)),
		"attempt", attempt,
	}
	if err != nil {
		kv = append(kv, "error", err.Error())
		t.log.Info("cli request failed", kv...)
		return
	}
	kv = append(kv,
		"status", status,
		"httpStatus", resp.StatusCode,
	)
	if status >= 400 {
		kv = append(kv, "body", peekBody(resp))
		t.log.Info("cli request failed", kv...)
		return
	}
	t.log.Debug("cli request ok", kv...)
}

// peekBody reads up to 1KiB for logging without consuming the stream, so
// downstream still gets the full body. The 500 body is the human-readable CLI error.
func peekBody(resp *http.Response) string {
	if resp.Body == nil {
		return ""
	}
	const max = 1 << 10
	br := bufio.NewReaderSize(resp.Body, max)
	resp.Body = &peekedBody{Reader: br, body: resp.Body}
	peeked, _ := br.Peek(max) // short read (EOF) is fine: we log what we got
	return string(peeked)
}

// peekedBody serves the buffered+unread bytes via the bufio.Reader while
// delegating Close to the original body.
type peekedBody struct {
	io.Reader
	body io.Closer
}

func (b *peekedBody) Close() error { return b.body.Close() }

func sessionFingerprint(id string) string {
	if id == "" {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(id)))[:12]
}
