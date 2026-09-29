package tfclient

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
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
)

// btpcli sets these on every data request once a session exists; eviction reads
// them off a 401'd request. Hard-coded because btpcli is internal/ and unimportable.
const (
	headerCLISessionId     = "X-Cpcli-Sessionid"
	headerCLISubdomain     = "X-Cpcli-Subdomain"
	headerCorrelationID    = "X-Correlationid"
	headerCLIBackendStatus = "X-Cpcli-Backend-Status"
)

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
	// hierarchy is nil when the hierarchy call is switched off, which keeps a
	// literal cliTransport without it behaving as before.
	hierarchy *hierarchyLoader
	// failFast is false in a literal cliTransport, which keeps handing the bare
	// 500 up as before.
	failFast bool
}

// RoundTrip absorbs the CLI server's bare 500 for a subaccount it has not loaded:
// if that 500 reached btpcli's retry layer, the retry chain would hold the session
// mutex and block every other request of the provider config for about a minute.
// Resending is safe because a bare 500 carries no backend status, so the command
// never reached the backend. A bare 500 that survives the hierarchy call and the
// resend is replaced with failFast, so it never reaches that retry layer either,
// unless the subaccount is guarded: a write to it was answered within the ttl, or
// a command was answered while the guard was up. Upstream create, update and
// delete poll with further commands after the write, at most 10 s apart by
// default, and failing such a poll fast would report a write that went through as
// failed and drop its state. Inside the guard the transport first resends a few
// times after short waits, each after a new hierarchy call, and only then hands
// the bare 500 up, so btpcli's retries carry the poll. Those retries keep the
// correlation id and are handed up at once, even after the guard ran out, so
// they neither repeat the waits nor end the chain in a fail-fast.
func (t *cliTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	cmd := t.hierarchy.parse(r)
	if cmd == nil {
		return t.send(r)
	}
	resp, sent, err := t.sendCommand(r, cmd)
	if !isBare500(resp, err) {
		if err == nil {
			t.markAnswered(cmd, resp)
		}
		return resp, err
	}
	t.hierarchy.forget(cmd.key, sent)
	if !t.failFast {
		return resp, err
	}
	corr := r.Header.Get(headerCorrelationID)
	ago, guarded, handedUp := t.hierarchy.guard(cmd.key, corr)
	if !guarded && !handedUp {
		return t.notLoadedResponse(r, cmd, resp), nil
	}
	resends := 0
	if !handedUp {
		resp, resends, err = t.resendGuarded(r, cmd, resp, sent)
		if err == nil && resp.Header.Get(headerCLIBackendStatus) != "" {
			return resp, nil
		}
		// btpcli retries a failed resend as it would the bare 500.
		t.hierarchy.markHandedUp(cmd.key, corr)
		if err != nil || !isBare500(resp, nil) {
			return resp, err
		}
	}
	if t.log != nil {
		t.log.Info("cli server refused a subaccount after a write, leaving the retries to btpcli",
			"cliServerURL", r.URL.Host,
			"subaccount", cmd.subaccount,
			"lastServedAgoMs", ago.Milliseconds(),
			"resends", resends,
			"correlationID", corr)
	}
	return resp, nil
}

// markAnswered ignores answers without a backend status: only the backend can
// have acted on a write. A write it rejected is reported as failed by upstream
// without polls, so it starts no guard.
func (t *cliTransport) markAnswered(cmd *cliCommand, resp *http.Response) bool {
	backend := resp.Header.Get(headerCLIBackendStatus)
	if backend == "" {
		return false
	}
	status, perr := strconv.Atoi(backend)
	t.hierarchy.markServed(cmd.key, cmd.write && (perr != nil || status < 400))
	return true
}

// resendGuarded returns the first answer that is no bare 500, or the last bare
// 500 when the waits run out or the hierarchy call fails, since a resend to a
// subaccount the server has not loaded is refused again.
func (t *cliTransport) resendGuarded(r *http.Request, cmd *cliCommand, resp *http.Response, sent time.Time) (*http.Response, int, error) {
	l := t.hierarchy
	resends := 0
	for _, wait := range l.resendWaits {
		if err := l.sleep(r.Context(), wait); err != nil {
			_ = resp.Body.Close()
			return nil, resends, err
		}
		if _, loaded := t.ensureLoaded(r, cmd, sent); !loaded {
			return resp, resends, nil
		}
		body, gerr := r.GetBody()
		if gerr != nil {
			return resp, resends, nil
		}
		_ = resp.Body.Close()
		r2 := r.Clone(r.Context())
		r2.Body = body
		sent = l.now()
		var err error
		resp, err = t.send(r2)
		resends++
		if !isBare500(resp, err) {
			if err == nil && t.markAnswered(cmd, resp) {
				if t.log != nil {
					t.log.Info("cli server served the subaccount after repeating the hierarchy call",
						"cliServerURL", r.URL.Host,
						"subaccount", cmd.subaccount,
						"resends", resends,
						"correlationID", r.Header.Get(headerCorrelationID))
				}
			}
			return resp, resends, err
		}
		l.forget(cmd.key, sent)
	}
	return resp, resends, nil
}

// sendCommand also returns when the answer's request was sent.
func (t *cliTransport) sendCommand(r *http.Request, cmd *cliCommand) (*http.Response, time.Time, error) {
	called, loaded := t.ensureLoaded(r, cmd, t.hierarchy.now().Add(-t.hierarchy.ttl))
	sent := t.hierarchy.now()
	resp, err := t.send(r)
	if !loaded || called || !isBare500(resp, err) {
		return resp, sent, err
	}
	// The remembered call was stale; repeat it unless someone did so after our send.
	if _, loaded = t.ensureLoaded(r, cmd, sent); !loaded {
		return resp, sent, err
	}
	body, gerr := r.GetBody()
	if gerr != nil {
		return resp, sent, err
	}
	_ = resp.Body.Close()
	r2 := r.Clone(r.Context())
	r2.Body = body
	sent = t.hierarchy.now()
	resp, err = t.send(r2)
	return resp, sent, err
}

func (t *cliTransport) send(r *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.base.RoundTrip(r)
	t.logResult(r, resp, err, time.Since(start))
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

// logResult logs failed CLI calls at Info; success is silent and allocation-free.
// The CLI proxy almost always returns HTTP 200 - the real status is in X-Cpcli-Backend-Status
// Both transport status and that header are checked.
// The body carries the CLI server's human-readable error, which upjet's newTFError swallows.
func (t *cliTransport) logResult(r *http.Request, resp *http.Response, err error, d time.Duration) {
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
	if err == nil && status < 400 {
		return
	}
	kv := []interface{}{
		"method", r.Method,
		"url", r.URL.Host + r.URL.Path,
		"cliServerURL", r.URL.Host,
		"durationMs", d.Milliseconds(),
	}
	if err != nil {
		kv = append(kv, "error", err.Error())
	} else {
		kv = append(kv,
			"status", status,
			"httpStatus", resp.StatusCode,
			"correlationID", resp.Header.Get(headerCorrelationID),
			"body", peekBody(resp))
	}
	t.log.Info("cli request failed", kv...)
}

// peekBody reads up to 1KiB for logging without consuming the stream, so
// downstream still gets the full body. The 500 body is the human-readable CLI error.
func peekBody(resp *http.Response) string {
	peeked, _ := peekBodyErr(resp) // short read (EOF) is fine: we log what we got
	return peeked
}

func peekBodyErr(resp *http.Response) (string, error) {
	if resp.Body == nil {
		return "", nil
	}
	const max = 1 << 10
	br := bufio.NewReaderSize(resp.Body, max)
	resp.Body = &peekedBody{Reader: br, body: resp.Body}
	peeked, err := br.Peek(max)
	return string(peeked), err
}

// peekedBody serves the buffered+unread bytes via the bufio.Reader while
// delegating Close to the original body.
type peekedBody struct {
	io.Reader
	body io.Closer
}

func (b *peekedBody) Close() error { return b.body.Close() }
