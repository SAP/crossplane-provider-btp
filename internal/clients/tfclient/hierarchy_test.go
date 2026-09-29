package tfclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
)

// The protocol as measured against a real CLI server. Written out instead of
// taken from the production constants, so a typo there fails these tests.
const (
	hierWireHierarchyPath = "/client/v2.106.1/globalAccountHierarchyForNodes"
	hierWireFormat        = "X-Cpcli-Format"
	hierWireSessionID     = "X-Cpcli-Sessionid"
	hierWireSubdomain     = "X-Cpcli-Subdomain"
	hierWireCustomIDP     = "X-Cpcli-Customidp"
	hierWireCorrelationID = "X-Correlationid"
	hierWireBackendStatus = "X-Cpcli-Backend-Status"
)

const (
	hierSubaccount  = "11111111-2222-4333-8444-555555555555"
	hierSubaccount2 = "66666666-7777-4888-8999-aaaaaaaaaaaa"
	hierSession     = "hier-session-0123456789abcdef"
	hierHost        = "cli.example.test"
	hierCommandURL  = "https://" + hierHost + "/command/v2.106.1/services/instance?get"
	hierHierURL     = "https://" + hierHost + hierWireHierarchyPath
)

func hierBody(subaccount string) string {
	return `{"paramValues":{"id":"abc","parameters":"false","subaccount":"` + subaccount + `"}}`
}

// hierCommand builds a request shaped like btpcli's doRequest, so GetBody is set.
func hierCommand(t *testing.T, url, session, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "hier-agent/1.0")
	req.Header.Set(hierWireFormat, "json")
	if session != "" {
		req.Header.Set(hierWireSessionID, session)
		req.Header.Set(hierWireSubdomain, "hier-ga")
		req.Header.Set(hierWireCustomIDP, "")
	}
	return req
}

func hierResp(status int, backend, body string) *http.Response {
	h := http.Header{}
	if backend != "" {
		h.Set(hierWireBackendStatus, backend)
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body))}
}

func hierBare500() *http.Response {
	return &http.Response{StatusCode: 500, Header: http.Header{}, Body: http.NoBody}
}

type hierClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *hierClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *hierClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// hierBase is the fake CLI server seen from below cliTransport.
type hierBase struct {
	mu          sync.Mutex
	hierReqs    []*http.Request
	hierBodies  []string
	commands    int
	onHierarchy func() (*http.Response, error)
	onCommand   func() (*http.Response, error)
}

func (b *hierBase) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.HasSuffix(r.URL.Path, hierWireHierarchyPath) {
		body, _ := io.ReadAll(r.Body)
		b.mu.Lock()
		b.hierReqs = append(b.hierReqs, r)
		b.hierBodies = append(b.hierBodies, string(body))
		f := b.onHierarchy
		b.mu.Unlock()
		if f != nil {
			return f()
		}
		return hierResp(200, "200", `{"guid":"ga"}`), nil
	}
	_, _ = io.ReadAll(r.Body)
	b.mu.Lock()
	b.commands++
	f := b.onCommand
	b.mu.Unlock()
	if f != nil {
		return f()
	}
	return hierResp(200, "200", `{"ok":true}`), nil
}

func (b *hierBase) counts() (hierarchy, commands int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.hierReqs), b.commands
}

func (b *hierBase) setCommand(f func() (*http.Response, error)) {
	b.mu.Lock()
	b.onCommand = f
	b.mu.Unlock()
}

type hierLogEntry struct {
	level string
	msg   string
	kv    []interface{}
}

// hierLogger records every Info and Debug call; capLogger keeps only the last Info.
type hierLogger struct {
	mu      sync.Mutex
	entries []hierLogEntry
}

func (l *hierLogger) Info(msg string, kv ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, hierLogEntry{"info", msg, kv})
}
func (l *hierLogger) Debug(msg string, kv ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, hierLogEntry{"debug", msg, kv})
}
func (l *hierLogger) WithValues(...interface{}) logging.Logger { return l }

func (l *hierLogger) find(level, msg string) []hierLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []hierLogEntry
	for _, e := range l.entries {
		if e.level == level && (msg == "" || e.msg == msg) {
			out = append(out, e)
		}
	}
	return out
}

// hierTransport returns a transport with the hierarchy call on and a fake clock.
func hierTransport(base http.RoundTripper, log logging.Logger) (*cliTransport, *hierClock) {
	clk := &hierClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	tr := &cliTransport{
		base:      base,
		evictSub:  func(string) {},
		evictAll:  func() {},
		log:       log,
		hierarchy: newHierarchyLoader(),
	}
	tr.hierarchy.now = clk.now
	return tr, clk
}

func hierRoundTrip(t *testing.T, tr *cliTransport, req *http.Request) (int, string) {
	t.Helper()
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestHierarchyParse(t *testing.T) {
	valid := hierBody(hierSubaccount)
	big := `{"paramValues":{"subaccount":"` + hierSubaccount + `"},"pad":"` + strings.Repeat("x", maxCommandBodyBytes) + `"}`

	nilCases := []struct {
		name      string
		req       func(t *testing.T) *http.Request
		nilLoader bool
	}{
		{name: "login POST", req: func(t *testing.T) *http.Request {
			return hierCommand(t, "https://"+hierHost+"/login/v2.106.1", "", valid)
		}},
		{name: "command without session header", req: func(t *testing.T) *http.Request {
			return hierCommand(t, hierCommandURL, "", valid)
		}},
		{name: "GET", req: func(t *testing.T) *http.Request {
			r := hierCommand(t, hierCommandURL, hierSession, valid)
			r.Method = http.MethodGet
			return r
		}},
		{name: "path without /command/", req: func(t *testing.T) *http.Request {
			return hierCommand(t, "https://"+hierHost+"/client/v2.106.1/services/instance", hierSession, valid)
		}},
		{name: "command path without command", req: func(t *testing.T) *http.Request {
			return hierCommand(t, "https://"+hierHost+"/command/v2.106.1", hierSession, valid)
		}},
		{name: "GetBody nil", req: func(t *testing.T) *http.Request {
			r := hierCommand(t, hierCommandURL, hierSession, valid)
			r.GetBody = nil
			return r
		}},
		{name: "invalid JSON", req: func(t *testing.T) *http.Request {
			return hierCommand(t, hierCommandURL, hierSession, `{"paramValues":`)
		}},
		{name: "empty body", req: func(t *testing.T) *http.Request {
			return hierCommand(t, hierCommandURL, hierSession, "")
		}},
		{name: "no subaccount", req: func(t *testing.T) *http.Request {
			return hierCommand(t, hierCommandURL, hierSession, `{"paramValues":{"id":"abc"}}`)
		}},
		{name: "directory only", req: func(t *testing.T) *http.Request {
			return hierCommand(t, hierCommandURL, hierSession, `{"paramValues":{"directory":"`+hierSubaccount+`"}}`)
		}},
		{name: "globalAccount only", req: func(t *testing.T) *http.Request {
			return hierCommand(t, hierCommandURL, hierSession, `{"paramValues":{"globalAccount":"`+hierSubaccount+`"}}`)
		}},
		{name: "subaccountID", req: func(t *testing.T) *http.Request {
			return hierCommand(t, hierCommandURL, hierSession, `{"paramValues":{"subaccountID":"`+hierSubaccount+`"}}`)
		}},
		{name: "subaccountFilter", req: func(t *testing.T) *http.Request {
			return hierCommand(t, hierCommandURL, hierSession, `{"paramValues":{"subaccountFilter":"`+hierSubaccount+`"}}`)
		}},
		{name: "subaccount not a string", req: func(t *testing.T) *http.Request {
			return hierCommand(t, hierCommandURL, hierSession, `{"paramValues":{"subaccount":123}}`)
		}},
		{name: "body over 1 MiB", req: func(t *testing.T) *http.Request {
			return hierCommand(t, hierCommandURL, hierSession, big)
		}},
		{name: "nil loader", nilLoader: true, req: func(t *testing.T) *http.Request {
			return hierCommand(t, hierCommandURL, hierSession, valid)
		}},
	}
	for _, tc := range nilCases {
		t.Run(tc.name, func(t *testing.T) {
			l := newHierarchyLoader()
			if tc.nilLoader {
				l = nil
			}
			if got := l.parse(tc.req(t)); got != nil {
				t.Fatalf("parse = %+v, want nil", got)
			}
		})
	}

	okCases := []struct {
		name, url, wantHierURL string
	}{
		{"plain", hierCommandURL, hierHierURL},
		{"path prefix", "https://" + hierHost + "/proxy/cli/command/v2.106.1/services/instance?get",
			"https://" + hierHost + "/proxy/cli" + hierWireHierarchyPath},
	}
	for _, tc := range okCases {
		t.Run(tc.name, func(t *testing.T) {
			req := hierCommand(t, tc.url, hierSession, valid)
			got := newHierarchyLoader().parse(req)
			if got == nil {
				t.Fatal("parse = nil, want command")
			}
			if got.subaccount != hierSubaccount {
				t.Errorf("subaccount = %q", got.subaccount)
			}
			if got.key != hierHost+"/"+hierSubaccount {
				t.Errorf("key = %q", got.key)
			}
			if got.hierarchyURL != tc.wantHierURL {
				t.Errorf("hierarchyURL = %q, want %q", got.hierarchyURL, tc.wantHierURL)
			}
			body, _ := io.ReadAll(req.Body)
			if string(body) != valid {
				t.Errorf("request body after parse = %q, want %q", body, valid)
			}
		})
	}
}

func TestHierarchyIsBare500(t *testing.T) {
	cases := []struct {
		name string
		resp *http.Response
		err  error
		want bool
	}{
		{"500 empty body no cli header", hierBare500(), nil, true},
		{"500 nil body", &http.Response{StatusCode: 500, Header: http.Header{}}, nil, true},
		{"500 with body", hierResp(500, "", "server trouble"), nil, false},
		{"500 with backend status", hierResp(500, "500", ""), nil, false},
		{"502", &http.Response{StatusCode: 502, Header: http.Header{}, Body: http.NoBody}, nil, false},
		{"200 with backend 404", hierResp(200, "404", ""), nil, false},
		{"error with nil response", nil, errors.New("dial fail"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBare500(tc.resp, tc.err); got != tc.want {
				t.Fatalf("isBare500 = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHierarchyRequestShape(t *testing.T) {
	for _, corr := range []string{"hier-corr-1", ""} {
		t.Run(fmt.Sprintf("correlation %q", corr), func(t *testing.T) {
			base := &hierBase{}
			tr, _ := hierTransport(base, nil)
			req := hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount))
			if corr != "" {
				req.Header.Set(hierWireCorrelationID, corr)
			}
			if status, _ := hierRoundTrip(t, tr, req); status != 200 {
				t.Fatalf("status = %d", status)
			}
			if len(base.hierReqs) != 1 {
				t.Fatalf("hierarchy calls = %d, want 1", len(base.hierReqs))
			}
			h := base.hierReqs[0]
			if h.Method != http.MethodPost {
				t.Errorf("method = %s", h.Method)
			}
			if h.URL.String() != hierHierURL {
				t.Errorf("url = %s, want %s", h.URL, hierHierURL)
			}
			wantBody := `{"nodes":[{"entityGuid":"` + hierSubaccount + `","entityType":"SUBACCOUNT"}]}`
			if base.hierBodies[0] != wantBody {
				t.Errorf("body = %s, want %s", base.hierBodies[0], wantBody)
			}
			for k, want := range map[string]string{
				"Content-Type":    "application/json",
				hierWireFormat:    "json",
				hierWireSessionID: hierSession,
				"User-Agent":      "hier-agent/1.0",
			} {
				if got := h.Header.Get(k); got != want {
					t.Errorf("%s = %q, want %q", k, got, want)
				}
			}
			for _, k := range []string{hierWireSubdomain, hierWireCustomIDP} {
				if v, ok := h.Header[k]; !ok || len(v) != 1 || v[0] != "" {
					t.Errorf("%s = %v (present %v), want present and empty", k, v, ok)
				}
			}
			got := h.Header.Get(hierWireCorrelationID)
			if corr != "" && got != corr {
				t.Errorf("correlation id = %q, want %q", got, corr)
			}
			if got == "" {
				t.Error("correlation id is empty")
			}
		})
	}
}

func TestHierarchyCallFailureFailsOpen(t *testing.T) {
	cases := []struct {
		name string
		hier func() (*http.Response, error)
	}{
		{"401", func() (*http.Response, error) { return hierResp(401, "", "unauthorized"), nil }},
		{"403", func() (*http.Response, error) { return hierResp(403, "", "forbidden"), nil }},
		{"500", func() (*http.Response, error) { return hierBare500(), nil }},
		{"200 with backend 404", func() (*http.Response, error) { return hierResp(200, "404", "not found"), nil }},
		{"200 without backend status", func() (*http.Response, error) { return hierResp(200, "", "{}"), nil }},
		{"transport error", func() (*http.Response, error) { return nil, errors.New("dial fail") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := &hierBase{onHierarchy: tc.hier}
			log := &hierLogger{}
			tr, _ := hierTransport(base, log)
			var evicted bool
			tr.evictSub = func(string) { evicted = true }
			tr.evictAll = func() { evicted = true }

			status, body := hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
			if status != 200 || body != `{"ok":true}` {
				t.Fatalf("command answer = %d %q, want it unchanged", status, body)
			}
			if h, c := base.counts(); h != 1 || c != 1 {
				t.Fatalf("hierarchy=%d commands=%d, want 1/1", h, c)
			}
			if evicted {
				t.Fatal("a failed hierarchy call must not evict a session")
			}
			if len(log.find("info", "")) == 0 {
				t.Fatal("failed hierarchy call was not logged at Info")
			}

			hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
			if h, _ := base.counts(); h != 2 {
				t.Fatalf("hierarchy calls after a failure = %d, want 2", h)
			}
		})
	}
}

// Each attempt of btpcli's retry layer re-enters RoundTrip; when the hierarchy
// call does not help, an attempt must cost one call and one command, not a resend.
func TestHierarchyNoLoop(t *testing.T) {
	base := &hierBase{onCommand: func() (*http.Response, error) { return hierBare500(), nil }}
	log := &hierLogger{}
	tr, clk := hierTransport(base, log)

	for attempt := 1; attempt <= 3; attempt++ {
		status, _ := hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
		if h, c := base.counts(); status != 500 || h != attempt || c != attempt {
			t.Fatalf("attempt %d: status=%d hierarchy=%d commands=%d, want 500/%d/%d", attempt, status, h, c, attempt, attempt)
		}
		clk.advance(time.Second)
	}
	if n := len(log.find("info", "cli server had not loaded the subaccount, repeating hierarchy call")); n != 0 {
		t.Fatalf("repeat log lines = %d, want 0: the disproved call must be forgotten", n)
	}
}

// A remembered call disproved by a bare 500 whose repeat fails is forgotten too.
func TestHierarchyStaleCallForgottenWhenRepeatFails(t *testing.T) {
	base := &hierBase{}
	tr, clk := hierTransport(base, nil)
	hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))

	base.setCommand(func() (*http.Response, error) { return hierBare500(), nil })
	base.mu.Lock()
	base.onHierarchy = func() (*http.Response, error) { return hierResp(401, "", "unauthorized"), nil }
	base.mu.Unlock()
	clk.advance(time.Second)
	status, _ := hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
	if h, c := base.counts(); status != 500 || h != 2 || c != 2 {
		t.Fatalf("status=%d hierarchy=%d commands=%d, want 500/2/2", status, h, c)
	}

	base.mu.Lock()
	base.onHierarchy = nil
	base.mu.Unlock()
	clk.advance(time.Second)
	hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
	if h, c := base.counts(); h != 3 || c != 3 {
		t.Fatalf("next attempt: hierarchy=%d commands=%d, want 3/3 (call first, no resend)", h, c)
	}
}

// A newer successful call must survive the bare 500 of an older send.
func TestHierarchyForgetKeepsNewerCall(t *testing.T) {
	l := newHierarchyLoader()
	key := hierHost + "/" + hierSubaccount
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l.entries[key] = &hierarchyEntry{loadedAt: t0.Add(time.Second)}
	l.forget(key, t0)
	if got := l.entries[key].loadedAt; !got.Equal(t0.Add(time.Second)) {
		t.Fatalf("loadedAt = %v, want the newer call kept", got)
	}
	l.forget(key, t0.Add(time.Second))
	if got := l.entries[key].loadedAt; !got.IsZero() {
		t.Fatalf("loadedAt = %v, want forgotten", got)
	}
}

func TestHierarchyForgottenSubaccountIsReloaded(t *testing.T) {
	base := &hierBase{}
	log := &hierLogger{}
	tr, clk := hierTransport(base, log)
	hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))

	// The server drops the subaccount before the TTL and answers the next command
	// with a bare 500 once.
	var served500 int
	base.setCommand(func() (*http.Response, error) {
		if served500 == 0 {
			served500++
			return hierBare500(), nil
		}
		return hierResp(200, "200", `{"ok":true}`), nil
	})
	clk.advance(30 * time.Second)
	status, body := hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
	if status != 200 || body != `{"ok":true}` {
		t.Fatalf("answer = %d %q, want 200", status, body)
	}
	if h, c := base.counts(); h != 2 || c != 3 || served500 != 1 {
		t.Fatalf("hierarchy=%d commands=%d bare500=%d, want 2/3/1", h, c, served500)
	}
	repeats := log.find("info", "cli server had not loaded the subaccount, repeating hierarchy call")
	if len(repeats) != 1 {
		t.Fatalf("repeat log lines = %d, want 1", len(repeats))
	}
}

func TestHierarchyTTL(t *testing.T) {
	base := &hierBase{}
	tr, clk := hierTransport(base, nil)
	send := func() {
		if status, _ := hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount))); status != 200 {
			t.Fatalf("status = %d", status)
		}
	}

	send()
	clk.advance(hierarchyCallTTL - time.Second)
	send()
	if h, _ := base.counts(); h != 1 {
		t.Fatalf("hierarchy calls within TTL = %d, want 1", h)
	}
	clk.advance(time.Second)
	send()
	if h, _ := base.counts(); h != 2 {
		t.Fatalf("hierarchy calls after TTL = %d, want 2", h)
	}
}

func TestHierarchyOtherErrorsAreNotResent(t *testing.T) {
	cases := []struct {
		name string
		resp func() *http.Response
	}{
		{"500 with body", func() *http.Response { return hierResp(500, "", "server trouble") }},
		{"502", func() *http.Response {
			return &http.Response{StatusCode: 502, Header: http.Header{}, Body: http.NoBody}
		}},
		{"200 with backend 404", func() *http.Response { return hierResp(200, "404", "not found") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := &hierBase{}
			tr, _ := hierTransport(base, nil)
			hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))

			base.setCommand(func() (*http.Response, error) { return tc.resp(), nil })
			want := tc.resp()
			status, _ := hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
			if status != want.StatusCode {
				t.Fatalf("status = %d, want %d", status, want.StatusCode)
			}
			if h, c := base.counts(); h != 1 || c != 2 {
				t.Fatalf("hierarchy=%d commands=%d, want 1/2", h, c)
			}
		})
	}
}

func TestHierarchyKeyIsServerAndSubaccount(t *testing.T) {
	base := &hierBase{}
	tr, _ := hierTransport(base, nil)
	otherURL := strings.Replace(hierCommandURL, hierHost, "cli2.example.test", 1)

	hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
	hierRoundTrip(t, tr, hierCommand(t, otherURL, hierSession, hierBody(hierSubaccount)))
	if h, _ := base.counts(); h != 2 {
		t.Fatalf("same subaccount on two hosts: hierarchy calls = %d, want 2", h)
	}
	if base.hierReqs[0].URL.Host != hierHost || base.hierReqs[1].URL.Host != "cli2.example.test" {
		t.Fatalf("hierarchy hosts = %s, %s", base.hierReqs[0].URL.Host, base.hierReqs[1].URL.Host)
	}

	hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount2)))
	if h, _ := base.counts(); h != 3 {
		t.Fatalf("second subaccount: hierarchy calls = %d, want 3", h)
	}

	hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, "hier-other-session", hierBody(hierSubaccount)))
	if h, _ := base.counts(); h != 3 {
		t.Fatalf("second session: hierarchy calls = %d, want 3", h)
	}
}

func TestHierarchyConcurrentCommandsShareOneCall(t *testing.T) {
	base := &hierBase{}
	tr, _ := hierTransport(base, nil)

	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		req := hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount))
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := tr.RoundTrip(req)
			if err != nil {
				errs <- err
				return
			}
			_ = resp.Body.Close()
			if resp.StatusCode != 200 {
				errs <- fmt.Errorf("status %d", resp.StatusCode)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if h, c := base.counts(); h != 1 || c != 50 {
		t.Fatalf("hierarchy=%d commands=%d, want 1/50", h, c)
	}
}

// hierSignalCtx reports the first Done call. A command waiting for another
// command's hierarchy call makes it; nothing on its path does so earlier.
type hierSignalCtx struct {
	context.Context
	once    sync.Once
	waiting chan struct{}
}

func (c *hierSignalCtx) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

// hierHangingBase answers the first hierarchy call with `result` only once
// released; `arrived` is closed when that call has reached it.
func hierHangingBase(t *testing.T, result func() (*http.Response, error)) (base *hierBase, arrived chan struct{}, release func()) {
	arrived, gate := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release = sync.OnceFunc(func() { close(gate) })
	t.Cleanup(release)
	base = &hierBase{onHierarchy: func() (*http.Response, error) {
		once.Do(func() { close(arrived) })
		<-gate
		return result()
	}}
	return base, arrived, release
}

func hierAsync(tr *cliTransport, req *http.Request) <-chan error {
	ch := make(chan error, 1)
	go func() {
		resp, err := tr.RoundTrip(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode != 200 {
				err = fmt.Errorf("status %d", resp.StatusCode)
			}
		}
		ch <- err
	}()
	return ch
}

func hierAwait(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out: %s", what)
	}
}

func hierAwaitErr(t *testing.T, ch <-chan error, what string) {
	t.Helper()
	select {
	case err := <-ch:
		if err != nil {
			t.Errorf("%s: %v", what, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out: %s", what)
	}
}

// Several provider configs hit one subaccount while its hierarchy endpoint hangs:
// they must wait for the one call in flight and share its failure, not queue up
// and each spend a full timeout on a call of their own.
func TestHierarchyWaitersShareFailedCallInFlight(t *testing.T) {
	base, arrived, release := hierHangingBase(t, func() (*http.Response, error) { return nil, context.DeadlineExceeded })
	tr, _ := hierTransport(base, nil)

	first := hierAsync(tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
	hierAwait(t, arrived, "first hierarchy call")

	ctx := &hierSignalCtx{Context: context.Background(), waiting: make(chan struct{})}
	second := hierAsync(tr, hierCommand(t, hierCommandURL, "hier-other-session", hierBody(hierSubaccount)).WithContext(ctx))
	hierAwait(t, ctx.waiting, "second command waiting for the call in flight")
	release()

	hierAwaitErr(t, first, "first command")
	hierAwaitErr(t, second, "second command")
	if h, c := base.counts(); h != 1 || c != 2 {
		t.Fatalf("hierarchy=%d commands=%d, want 1/2", h, c)
	}
}

func TestHierarchyWaiterHonoursCancellation(t *testing.T) {
	base, arrived, release := hierHangingBase(t, func() (*http.Response, error) { return hierResp(200, "200", "{}"), nil })
	tr, _ := hierTransport(base, nil)

	first := hierAsync(tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
	hierAwait(t, arrived, "first hierarchy call")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	second := hierAsync(tr, hierCommand(t, hierCommandURL, "hier-other-session", hierBody(hierSubaccount)).WithContext(ctx))
	hierAwaitErr(t, second, "cancelled command while the hierarchy call hangs")
	if h, _ := base.counts(); h != 1 {
		t.Fatalf("hierarchy calls = %d, want 1", h)
	}

	release()
	hierAwaitErr(t, first, "first command")
}

func TestHierarchySessionIDIsNeverLogged(t *testing.T) {
	scenarios := map[string]func(t *testing.T, base *hierBase, tr *cliTransport, clk *hierClock){
		"success": func(t *testing.T, _ *hierBase, tr *cliTransport, _ *hierClock) {
			hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
		},
		"repeat after bare 500": func(t *testing.T, base *hierBase, tr *cliTransport, clk *hierClock) {
			hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
			base.setCommand(func() (*http.Response, error) { return hierBare500(), nil })
			clk.advance(time.Second)
			hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
		},
		"failure 401": func(t *testing.T, base *hierBase, tr *cliTransport, _ *hierClock) {
			base.onHierarchy = func() (*http.Response, error) { return hierResp(401, "", "unauthorized"), nil }
			hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
		},
		"failure without backend status": func(t *testing.T, base *hierBase, tr *cliTransport, _ *hierClock) {
			base.onHierarchy = func() (*http.Response, error) { return hierResp(200, "", "{}"), nil }
			hierRoundTrip(t, tr, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
		},
	}
	for name, run := range scenarios {
		t.Run(name, func(t *testing.T) {
			base := &hierBase{}
			log := &hierLogger{}
			tr, clk := hierTransport(base, log)
			run(t, base, tr, clk)

			if len(log.entries) == 0 {
				t.Fatal("scenario logged nothing")
			}
			for _, e := range log.entries {
				if strings.Contains(e.msg, hierSession) || strings.Contains(fmt.Sprint(e.kv...), hierSession) {
					t.Fatalf("%s line %q leaks the session id: %v", e.level, e.msg, e.kv)
				}
			}
		})
	}
}

func TestHierarchySwitch(t *testing.T) {
	cp := &cachingProvider{entries: map[string]*cacheEntry{}}

	base := &hierBase{}
	off := newCLITransport(cp, base, nil, false, false)
	if off.hierarchy != nil {
		t.Fatal("switched off: hierarchy loader must be nil")
	}
	hierRoundTrip(t, off, hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount)))
	if h, c := base.counts(); h != 0 || c != 1 {
		t.Fatalf("switched off: hierarchy=%d commands=%d, want 0/1", h, c)
	}

	on := newCLITransport(cp, base, nil, true, false)
	if on.hierarchy == nil || on.hierarchy.ttl != hierarchyCallTTL || on.hierarchy.timeout != hierarchyCallTimeout {
		t.Fatalf("switched on: loader = %+v", on.hierarchy)
	}

	old := hierarchyCallEnabled
	t.Cleanup(func() { SetSubaccountHierarchyCall(old) })
	SetSubaccountHierarchyCall(false)
	if hierarchyCallEnabled {
		t.Fatal("SetSubaccountHierarchyCall(false) did not switch off")
	}
	SetSubaccountHierarchyCall(true)
	if !hierarchyCallEnabled {
		t.Fatal("SetSubaccountHierarchyCall(true) did not switch on")
	}
}
