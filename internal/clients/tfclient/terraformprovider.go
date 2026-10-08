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

	tfprovider "github.com/SAP/terraform-provider-btp/btp/provider"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/sap/crossplane-provider-btp/btp"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// btpcli sets these on every data request once a session exists; eviction reads
// them off a 401'd request. Hard-coded because btpcli is internal/ and unimportable.
const (
	headerCLISessionId     = "X-Cpcli-Sessionid"
	headerCLISubdomain     = "X-Cpcli-Subdomain"
	headerCorrelationID    = "X-Correlationid"
	headerCLIBackendStatus = "X-Cpcli-Backend-Status"
)

// terraformProvider integrates the Terraform BTP provider with Crossplane.
// It forwards framework capabilities and optionally reuses configured clients.
// Its CLI transport provides request logging and evicts shared sessions on 401.
type terraformProvider struct {
	fwprovider.Provider // forwards Metadata/Schema/Resources/DataSources

	reuseSessions bool

	mu      sync.Mutex
	entries map[string]*cacheEntry
}

// cacheEntry: once.Do collapses concurrent reconciles for the same key to one login.
type cacheEntry struct {
	once      sync.Once
	resp      *fwprovider.ConfigureResponse
	subdomain string // globalaccount from config; eviction index
}

func newTerraformProvider(inner fwprovider.Provider, reuseSessions bool) *terraformProvider {
	return &terraformProvider{
		Provider:      inner,
		reuseSessions: reuseSessions,
		entries:       map[string]*cacheEntry{},
	}
}

// newBTPFrameworkProvider always uses the integration wrapper and logging transport.
func newBTPFrameworkProvider(reuseSessions bool) fwprovider.Provider {
	p := newTerraformProvider(nil, reuseSessions)
	base := http.DefaultTransport
	if btp.IsDebug() {
		base = btp.DebugPrintHTTPClient().Transport
	}
	hc := &http.Client{Transport: &cliTransport{
		base:     base,
		evictSub: p.evictBySubdomain,
		evictAll: p.evictAll,
		log:      logging.NewLogrLogger(zap.New(zap.UseDevMode(btp.IsDebug())).WithName("crossplane-provider-btp-cli")),
	}}
	p.Provider = tfprovider.NewWithClient(hc)
	return p
}

func (p *terraformProvider) Configure(ctx context.Context, req fwprovider.ConfigureRequest, resp *fwprovider.ConfigureResponse) {
	if !p.reuseSessions {
		// Terraform Configure creates a new client/session in response data.
		// Each connector keeps its own response; nothing enters the shared cache.
		p.Provider.Configure(ctx, req, resp)
		return
	}

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
func (p *terraformProvider) Functions(ctx context.Context) []func() function.Function {
	if wf, ok := p.Provider.(fwprovider.ProviderWithFunctions); ok {
		return wf.Functions(ctx)
	}
	return nil
}

// ListResources forwards to the inner provider
func (p *terraformProvider) ListResources(ctx context.Context) []func() list.ListResource {
	if pl, ok := p.Provider.(fwprovider.ProviderWithListResources); ok {
		return pl.ListResources(ctx)
	}
	return nil
}

// Actions forwards to the inner provider
func (p *terraformProvider) Actions(ctx context.Context) []func() action.Action {
	if pa, ok := p.Provider.(fwprovider.ProviderWithActions); ok {
		return pa.Actions(ctx)
	}
	return nil
}

// evictBySubdomain drops cached sessions for a subdomain so the next reconcile re-authenticates.
func (p *terraformProvider) evictBySubdomain(subdomain string) {
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
func (p *terraformProvider) evictAll() {
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
