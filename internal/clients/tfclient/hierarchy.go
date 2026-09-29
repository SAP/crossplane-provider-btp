package tfclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// The CLI server answers a subaccount command with a bare HTTP 500 unless it has
// recently loaded that subaccount. The official btp CLI loads it with a hierarchy
// call before every command; terraform-provider-btp never does, so cliTransport does.
const (
	headerCLIFormat    = "X-Cpcli-Format"
	headerCLICustomIDP = "X-Cpcli-Customidp"
	headerCLIPrefix    = "X-Cpcli-"

	hierarchyEndpoint = "globalAccountHierarchyForNodes"
	// The server was seen to keep a subaccount loaded for at least 179 s; only
	// half of that is trusted.
	hierarchyCallTTL     = 90 * time.Second
	hierarchyCallTimeout = 10 * time.Second
	maxCommandBodyBytes  = 1 << 20

	// No resource reads backend status 500 as not found, forbidden or rate limited.
	notLoadedBackendStatus = "500"
)

var (
	hierarchyCallEnabled = true
	failFastEnabled      = true
)

// SetSubaccountHierarchyCall switches the hierarchy call on or off. It must be
// called before the first CLI client is built, i.e. from main().
func SetSubaccountHierarchyCall(enabled bool) { hierarchyCallEnabled = enabled }

// SetFailFastOnUnloadedSubaccount switches off btpcli's retries for a bare 500
// that survives the hierarchy call. It must be called before the first CLI
// client is built, i.e. from main().
func SetFailFastOnUnloadedSubaccount(enabled bool) { failFastEnabled = enabled }

type hierarchyLoader struct {
	ttl     time.Duration
	timeout time.Duration
	now     func() time.Time // replaced in tests

	mu      sync.Mutex
	entries map[string]*hierarchyEntry
}

// hierarchyEntry is guarded by hierarchyLoader.mu, which is never held across a
// hierarchy call.
type hierarchyEntry struct {
	loadedAt time.Time // start of the last successful hierarchy call
	servedAt time.Time // last answer to a command that carried a backend status
	// Concurrent commands share the outcome of the call in flight, failures
	// included, so a hanging endpoint costs them one timeout rather than one each.
	flight *hierarchyFlight
}

type hierarchyFlight struct {
	done chan struct{}
	ok   bool // written before done is closed
}

type cliCommand struct {
	key          string // r.URL.Host + "/" + subaccount
	subaccount   string
	hierarchyURL string
}

func newHierarchyLoader() *hierarchyLoader {
	return &hierarchyLoader{
		ttl:     hierarchyCallTTL,
		timeout: hierarchyCallTimeout,
		now:     time.Now,
		entries: map[string]*hierarchyEntry{},
	}
}

// parse returns nil for anything that is not a subaccount-scoped CLI command, so
// logins and all other requests pass through untouched.
func (l *hierarchyLoader) parse(r *http.Request) *cliCommand {
	if l == nil || r.Method != http.MethodPost || r.Header.Get(headerCLISessionId) == "" || r.GetBody == nil {
		return nil
	}
	prefix, rest, ok := strings.Cut(r.URL.Path, "/command/")
	if !ok {
		return nil
	}
	version, command, ok := strings.Cut(rest, "/")
	if !ok || version == "" || command == "" {
		return nil
	}

	// GetBody hands out a fresh copy, so r.Body stays intact for the real send.
	body, err := r.GetBody()
	if err != nil {
		return nil
	}
	defer body.Close() //nolint:errcheck
	raw, err := io.ReadAll(io.LimitReader(body, maxCommandBodyBytes+1))
	if err != nil || len(raw) > maxCommandBodyBytes {
		return nil
	}
	var parsed struct {
		ParamValues struct {
			Subaccount string `json:"subaccount"`
		} `json:"paramValues"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.ParamValues.Subaccount == "" {
		return nil
	}

	u := *r.URL
	u.Path = prefix + "/client/" + version + "/" + hierarchyEndpoint
	u.RawPath = ""
	u.RawQuery = ""
	sa := parsed.ParamValues.Subaccount
	return &cliCommand{
		key:          r.URL.Host + "/" + sa,
		subaccount:   sa,
		hierarchyURL: u.String(),
	}
}

// entryLocked must be called with l.mu held.
func (l *hierarchyLoader) entryLocked(key string) *hierarchyEntry {
	e, ok := l.entries[key]
	if !ok {
		e = &hierarchyEntry{}
		l.entries[key] = e
	}
	return e
}

// forget drops a remembered call that a bare 500 at `sent` has disproved, so the
// next attempt, a btpcli retry or with fail-fast the next reconcile, calls before
// sending instead of sending, calling and resending. A call that succeeded after
// `sent` is kept.
func (l *hierarchyLoader) forget(key string, sent time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e, ok := l.entries[key]; ok && !e.loadedAt.After(sent) {
		e.loadedAt = time.Time{}
	}
}

func (l *hierarchyLoader) markServed(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entryLocked(key).servedAt = l.now()
}

// servedAgo reports how long ago the backend last answered a command for key,
// and whether that was within the ttl the server is trusted to keep it loaded.
func (l *hierarchyLoader) servedAgo(key string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok || e.servedAt.IsZero() {
		return 0, false
	}
	ago := l.now().Sub(e.servedAt)
	return ago, ago < l.ttl
}

// ensureLoaded makes a hierarchy call unless one succeeded after `after`, or
// shares the outcome of the one already in flight.
func (t *cliTransport) ensureLoaded(r *http.Request, cmd *cliCommand, after time.Time) (called, loaded bool) {
	l := t.hierarchy
	l.mu.Lock()
	e := l.entryLocked(cmd.key)
	if e.loadedAt.After(after) {
		l.mu.Unlock()
		return false, true
	}
	if f := e.flight; f != nil {
		l.mu.Unlock()
		select {
		case <-f.done:
			return false, f.ok
		case <-r.Context().Done():
			return false, false
		}
	}
	f := &hierarchyFlight{done: make(chan struct{})}
	e.flight = f
	last := e.loadedAt
	l.mu.Unlock()

	start := l.now()
	if ago := start.Sub(last); !last.IsZero() && ago < l.ttl && t.log != nil {
		t.log.Info("cli server had not loaded the subaccount, repeating hierarchy call",
			"cliServerURL", r.URL.Host,
			"subaccount", cmd.subaccount,
			"lastHierarchyCallAgoMs", ago.Milliseconds())
	}
	f.ok = t.hierarchyCall(r, cmd)

	l.mu.Lock()
	if f.ok {
		e.loadedAt = start
	}
	e.flight = nil
	l.mu.Unlock()
	close(f.done)
	return true, f.ok
}

// hierarchyCall mirrors the request the official btp CLI sends. It goes through
// base, not send: a 401 here says nothing about the session's validity for
// commands and must not evict it.
func (t *cliTransport) hierarchyCall(r *http.Request, cmd *cliCommand) bool {
	ctx, cancel := context.WithTimeout(r.Context(), t.hierarchy.timeout)
	defer cancel()

	body, err := json.Marshal(map[string]any{
		"nodes": []map[string]string{{"entityGuid": cmd.subaccount, "entityType": "SUBACCOUNT"}},
	})
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cmd.hierarchyURL, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerCLIFormat, "json")
	req.Header.Set(headerCLISessionId, r.Header.Get(headerCLISessionId))
	// The btp CLI sends both empty; the server accepts the call that way.
	req.Header.Set(headerCLISubdomain, "")
	req.Header.Set(headerCLICustomIDP, "")
	corr := r.Header.Get(headerCorrelationID)
	if corr == "" {
		corr = uuid.NewString()
	}
	req.Header.Set(headerCorrelationID, corr)
	if ua := r.Header.Get("User-Agent"); ua != "" {
		req.Header.Set("User-Agent", ua)
	}

	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	d := time.Since(start)
	t.logResult(req, resp, err, d)
	if err != nil {
		return false
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}()

	backend := resp.Header.Get(headerCLIBackendStatus)
	ok := resp.StatusCode == http.StatusOK && backend == "200"
	if t.log == nil {
		return ok
	}
	if ok {
		t.log.Debug("cli hierarchy call succeeded",
			"cliServerURL", r.URL.Host,
			"subaccount", cmd.subaccount,
			"durationMs", d.Milliseconds())
		return ok
	}
	// logResult treats a sub-400 answer as success, but without backend status
	// 200 the server has not confirmed the subaccount is loaded.
	status := resp.StatusCode
	if b, e := strconv.Atoi(backend); e == nil {
		status = b
	}
	if status < 400 {
		t.log.Info("cli hierarchy call failed",
			"cliServerURL", r.URL.Host,
			"subaccount", cmd.subaccount,
			"httpStatus", resp.StatusCode,
			"backendStatus", backend,
			"durationMs", d.Milliseconds())
	}
	return ok
}

// notLoadedResponse replaces a bare 500 that survived the hierarchy call: btpcli
// would retry it for about a minute under the session mutex, stalling every other
// request of the provider config, while crossplane requeues the reconcile anyway.
// HTTP 200 with a backend status is not retried and is the only shape whose
// message btpcli passes on.
func (t *cliTransport) notLoadedResponse(r *http.Request, cmd *cliCommand, bare *http.Response) *http.Response {
	// A bare 500 carries no headers, so the request is the only source.
	corr := r.Header.Get(headerCorrelationID)
	msg := fmt.Sprintf("cli server has not loaded subaccount %s: it answered HTTP 500 without a backend status; not retried in-process", cmd.subaccount)
	if corr != "" {
		msg += fmt.Sprintf(" [Correlation ID: %s]", corr)
	}
	body, err := json.Marshal(struct {
		Error string `json:"error"`
	}{msg})
	if err != nil {
		return bare
	}
	if bare.Body != nil {
		_ = bare.Body.Close()
	}

	if t.log != nil {
		t.log.Info("cli server has not loaded the subaccount, failing the command without retries",
			"cliServerURL", r.URL.Host,
			"url", r.URL.Host+r.URL.Path,
			"subaccount", cmd.subaccount,
			"correlationID", corr)
	}

	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set(headerCLIBackendStatus, notLoadedBackendStatus)
	if corr != "" {
		h.Set(headerCorrelationID, corr)
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Proto:         bare.Proto,
		ProtoMajor:    bare.ProtoMajor,
		ProtoMinor:    bare.ProtoMinor,
		Header:        h,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       r,
	}
}

// isBare500 recognises the CLI server's "subaccount not loaded" answer: real
// backend errors always carry X-Cpcli-* headers, this one carries nothing.
func isBare500(resp *http.Response, err error) bool {
	if err != nil || resp == nil || resp.StatusCode != http.StatusInternalServerError || resp.ContentLength > 0 {
		return false
	}
	for k := range resp.Header {
		if strings.HasPrefix(http.CanonicalHeaderKey(k), headerCLIPrefix) {
			return false
		}
	}
	if resp.Body == nil {
		return true
	}
	// A body that fails to read may have held a real error; only a clean EOF is empty.
	peeked, perr := peekBodyErr(resp)
	return peeked == "" && errors.Is(perr, io.EOF)
}
