package tfclient

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
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

// poolConfig is the sizing/expiry policy for the session pool. size is the knob
// that spans both historical extremes and the new middle:
//
//	0  -> pool disabled; a fresh client per reconcile (old reuse=false).
//	1  -> one shared warmed session funnelling through its mutex (old reuse=true).
//	>=2 -> the pool.
type poolConfig struct {
	size int
	ttl  time.Duration
	now  func() time.Time // injectable clock; defaults to time.Now
}

func (c poolConfig) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// terraformProvider integrates the Terraform BTP provider with Crossplane. It
// forwards framework capabilities and keeps a bounded pool of warmed, reusable
// sessions per global-account key. Its CLI transport logs requests, captures each
// slot's session id, and marks a slot dirty when its session is terminated (401).
type terraformProvider struct {
	fwprovider.Provider // forwards Metadata/Schema/Resources/DataSources

	cfg poolConfig

	mu       sync.Mutex
	pools    map[string]*sessionPool // keyed by raw provider config (= GA creds)
	sessions map[string]*sessionSlot // session id -> slot, for 401 dirty lookup
}

// sessionPool is the per-GA ring of warmed sessions handed out round-robin.
type sessionPool struct {
	subdomain string
	next      atomic.Uint64
	slots     []*sessionSlot
}

func (p *sessionPool) pick() *sessionSlot {
	i := p.next.Add(1) - 1
	return p.slots[int(i%uint64(len(p.slots)))]
}

// sessionSlot holds one warmed session. once logged in, resp is reused until the
// slot is dirty (server killed the session) or past its TTL, when it re-warms.
type sessionSlot struct {
	subdomain string

	mu        sync.Mutex
	resp      *fwprovider.ConfigureResponse
	createdAt time.Time
	sessionID string
	dirty     atomic.Bool
}

func newTerraformProvider(inner fwprovider.Provider, cfg poolConfig) *terraformProvider {
	return &terraformProvider{
		Provider: inner,
		cfg:      cfg,
		pools:    map[string]*sessionPool{},
		sessions: map[string]*sessionSlot{},
	}
}

// newBTPFrameworkProvider always uses the integration wrapper and logging transport.
func newBTPFrameworkProvider(cfg poolConfig) fwprovider.Provider {
	p := newTerraformProvider(nil, cfg)
	var base http.RoundTripper
	if btp.IsDebug() {
		base = btp.DebugPrintHTTPClient().Transport
	} else {
		base = tunedTransport(cfg.size)
	}
	hc := &http.Client{Transport: &cliTransport{
		base:      instrumentedTransport(base),
		capture:   p.recordSessionFromCtx,
		markDirty: p.markDirtyBySession,
		log:       logging.NewLogrLogger(zap.New(zap.UseDevMode(btp.IsDebug())).WithName("crossplane-provider-btp-cli")),
	}}
	p.Provider = tfprovider.NewWithClient(hc)
	return p
}

// tunedTransport clones DefaultTransport and lifts MaxIdleConnsPerHost to the pool
// size so N concurrent sessions keep N keep-alive connections instead of churning
// TLS handshakes against DefaultTransport's per-host idle cap of 2.
func tunedTransport(size int) http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	n := size
	if n < 2 {
		n = 2
	}
	t.MaxIdleConnsPerHost = n
	if t.MaxIdleConns < n {
		t.MaxIdleConns = n
	}
	return t
}

func (p *terraformProvider) Configure(ctx context.Context, req fwprovider.ConfigureRequest, resp *fwprovider.ConfigureResponse) {
	if p.cfg.size <= 0 {
		// Pool disabled: Terraform Configure creates a new client/session per
		// reconcile. Nothing is cached.
		p.Provider.Configure(ctx, req, resp)
		return
	}

	// Raw config as key: schema-agnostic, so it can't drift when the provider
	// adds an attribute, and it covers every field a login branches on.
	key := fmt.Sprintf("%v", req.Config.Raw)
	pool := p.poolFor(ctx, req, key)
	p.warm(ctx, req, pool.pick(), resp)
}

func (p *terraformProvider) poolFor(ctx context.Context, req fwprovider.ConfigureRequest, key string) *sessionPool {
	p.mu.Lock()
	defer p.mu.Unlock()
	pool, ok := p.pools[key]
	if ok {
		return pool
	}
	var sd types.String
	req.Config.GetAttribute(ctx, path.Root("globalaccount"), &sd)
	subdomain := sd.ValueString()
	pool = &sessionPool{subdomain: subdomain, slots: make([]*sessionSlot, p.cfg.size)}
	for i := range pool.slots {
		pool.slots[i] = &sessionSlot{subdomain: subdomain}
	}
	p.pools[key] = pool
	poolSize.WithLabelValues(subdomain).Set(float64(p.cfg.size))
	return pool
}

// warm returns the slot's cached session, logging in (or re-logging-in) when the
// slot is empty, dirty, or past its TTL. The login ctx carries the slot so the CLI
// transport can bind the new session id to it.
func (p *terraformProvider) warm(ctx context.Context, req fwprovider.ConfigureRequest, s *sessionSlot, resp *fwprovider.ConfigureResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.resp != nil && !s.dirty.Load() && !p.expired(s) {
		poolCheckouts.WithLabelValues(s.subdomain).Inc()
		assignResp(resp, s.resp)
		return
	}

	if s.resp != nil { // replacing an existing session: record why
		reason := "ttl"
		if s.dirty.Load() {
			reason = "dirty"
		}
		poolEvictions.WithLabelValues(s.subdomain, reason).Inc()
	}
	p.forgetSession(s)

	var out fwprovider.ConfigureResponse
	p.Provider.Configure(withSlot(ctx, s), req, &out)
	if out.Diagnostics.HasError() {
		// Surface the error and leave the slot empty so the next reconcile retries.
		s.resp = nil
		resp.Diagnostics = out.Diagnostics
		return
	}
	// Copy without Diagnostics so a later hit doesn't replay stale warnings.
	s.resp = &fwprovider.ConfigureResponse{
		ResourceData:          out.ResourceData,
		DataSourceData:        out.DataSourceData,
		ListResourceData:      out.ListResourceData,
		ActionData:            out.ActionData,
		EphemeralResourceData: out.EphemeralResourceData,
	}
	s.createdAt = p.cfg.clock()
	if s.dirty.CompareAndSwap(true, false) {
		poolDirty.WithLabelValues(s.subdomain).Dec()
	}
	poolLogins.WithLabelValues(s.subdomain).Inc()
	assignResp(resp, s.resp)
}

func assignResp(dst, src *fwprovider.ConfigureResponse) {
	dst.ResourceData = src.ResourceData
	dst.DataSourceData = src.DataSourceData
	dst.ListResourceData = src.ListResourceData
	dst.ActionData = src.ActionData
	dst.EphemeralResourceData = src.EphemeralResourceData
}

func (p *terraformProvider) expired(s *sessionSlot) bool {
	if p.cfg.ttl <= 0 {
		return false
	}
	return p.cfg.clock().Sub(s.createdAt) > p.cfg.ttl
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

// slotCtxKey tags a login ctx with the slot it is warming.
type slotCtxKey struct{}

func withSlot(ctx context.Context, s *sessionSlot) context.Context {
	return context.WithValue(ctx, slotCtxKey{}, s)
}

func slotFromCtx(ctx context.Context) *sessionSlot {
	s, _ := ctx.Value(slotCtxKey{}).(*sessionSlot)
	return s
}

// recordSessionFromCtx binds the session id from a login response to the slot that
// triggered the login (carried on the request ctx), indexing it for 401 lookup.
func (p *terraformProvider) recordSessionFromCtx(ctx context.Context, resp *http.Response) {
	s := slotFromCtx(ctx)
	if s == nil || resp == nil {
		return
	}
	id := resp.Header.Get(headerCLISessionId)
	if id == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if s.sessionID != "" {
		delete(p.sessions, s.sessionID)
	}
	s.sessionID = id
	p.sessions[id] = s
}

func (p *terraformProvider) forgetSession(s *sessionSlot) {
	if s.sessionID == "" {
		return
	}
	p.mu.Lock()
	delete(p.sessions, s.sessionID)
	p.mu.Unlock()
	s.sessionID = ""
}

// markDirtyBySession flags the slot owning sessionID so its dead session is dropped
// and re-warmed on the next checkout. In-flight ops keep their own reference; the
// session is never logged out here (GC-only reclaim).
func (p *terraformProvider) markDirtyBySession(sessionID string) {
	p.mu.Lock()
	s := p.sessions[sessionID]
	p.mu.Unlock()
	if s == nil {
		return
	}
	if s.dirty.CompareAndSwap(false, true) {
		poolDirty.WithLabelValues(s.subdomain).Inc()
		poolEvictions.WithLabelValues(s.subdomain, "dirty").Inc()
	}
}

// cliTransport is the single injection point for all CLI/TF requests. It binds a
// slot's session id on login, marks a slot dirty on a 401 (so a terminated session
// can't be reused), and logs failures with enough context to diagnose a CLI-server
// outage.
type cliTransport struct {
	base      http.RoundTripper
	capture   func(ctx context.Context, resp *http.Response)
	markDirty func(sessionID string)
	log       logging.Logger
}

func (t *cliTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.base.RoundTrip(r)
	t.logResult(r, resp, err, time.Since(start))

	if t.capture != nil {
		t.capture(r.Context(), resp)
	}
	// The session-id check skips login POSTs (no session headers), so a bad-creds
	// login 401 can't evict a valid entry.
	if t.markDirty != nil && resp != nil && resp.StatusCode == http.StatusUnauthorized && r.Header.Get(headerCLISessionId) != "" {
		t.markDirty(r.Header.Get(headerCLISessionId))
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
