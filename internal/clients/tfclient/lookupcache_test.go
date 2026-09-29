package tfclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"
)

const (
	lookupWireMediaType = "X-Cpcli-Backend-Mediatype"
	lookupPlanURL       = "https://" + hierHost + "/command/v2.106.1/services/plan?get"
	lookupOfferingURL   = "https://" + hierHost + "/command/v2.106.1/services/offering?get"
	lookupPlanID        = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	lookupPlanAnswer    = `{"id":"` + lookupPlanID + `","name":"lookup-plan","service_offering_id":"bbbbbbbb-cccc-4ddd-8eee-ffffffffffff"}`
	lookupHitMessage    = "cli lookup served from cache"
	lookupFullMessage   = "cli lookup cache is full, dropping the oldest answers"
)

func lookupBody(subaccount, id string) string {
	return `{"paramValues":{"id":"` + id + `","subaccount":"` + subaccount + `"}}`
}

func lookupReq(t *testing.T, url, id, corr string) *http.Request {
	t.Helper()
	req := hierCommand(t, url, hierSession, lookupBody(hierSubaccount, id))
	if corr != "" {
		req.Header.Set(hierWireCorrelationID, corr)
	}
	return req
}

// lookupTransport returns a transport with the hierarchy call and a one-hour
// lookup cache on, both on the same fake clock.
func lookupTransport(base http.RoundTripper, log *hierLogger) (*cliTransport, *hierClock) {
	var tr *cliTransport
	var clk *hierClock
	if log == nil {
		tr, clk = hierTransport(base, nil)
		tr.lookups = newLookupCache(time.Hour, nil)
	} else {
		tr, clk = hierTransport(base, log)
		tr.lookups = newLookupCache(time.Hour, log)
	}
	tr.lookups.now = clk.now
	return tr, clk
}

func lookupPlanResp() (*http.Response, error) {
	resp := hierResp(200, "200", lookupPlanAnswer)
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Set(lookupWireMediaType, "application/json")
	resp.Header.Set(hierWireCorrelationID, "stored-corr")
	resp.Header.Set(hierWireSessionID, hierSession)
	resp.Header.Set("Set-Cookie", "lookup=1")
	return resp, nil
}

// lookupCall does one RoundTrip and returns the answer with its body read.
func lookupCall(t *testing.T, tr *cliTransport, req *http.Request) (*http.Response, string) {
	t.Helper()
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, string(body)
}

func lookupCommands(t *testing.T, b *hierBase) int {
	t.Helper()
	_, n := b.counts()
	return n
}

func TestLookupCacheParse(t *testing.T) {
	c := newLookupCache(time.Hour, nil)
	valid := lookupBody(hierSubaccount, lookupPlanID)

	cacheable := map[string]string{
		"plan":        lookupPlanURL,
		"offering":    lookupOfferingURL,
		"path prefix": "https://" + hierHost + "/cli/prefix/command/v2.106.1/services/plan?get",
	}
	for name, url := range cacheable {
		t.Run(name, func(t *testing.T) {
			r := hierCommand(t, url, hierSession, valid)
			q := c.parse(r)
			if q == nil {
				t.Fatal("parse = nil, want cacheable")
			}
			if q.subaccount != hierSubaccount || len(q.key) != 64 {
				t.Errorf("parse = %+v, want subaccount %s and a hex SHA-256 key", q, hierSubaccount)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != valid {
				t.Errorf("r.Body after parse = %q, want it untouched", body)
			}
		})
	}

	withBody := func(body string) func(t *testing.T) *http.Request {
		return func(t *testing.T) *http.Request { return hierCommand(t, lookupPlanURL, hierSession, body) }
	}
	withURL := func(url string) func(t *testing.T) *http.Request {
		return func(t *testing.T) *http.Request { return hierCommand(t, url, hierSession, valid) }
	}
	cmdBase := "https://" + hierHost + "/command/v2.106.1/"
	nilCases := []struct {
		name     string
		req      func(t *testing.T) *http.Request
		nilCache bool
	}{
		{name: "list", req: withURL(cmdBase + "services/plan?list")},
		{name: "create", req: withURL(cmdBase + "services/plan?create")},
		{name: "offering list", req: withURL(cmdBase + "services/offering?list")},
		{name: "services/instance", req: withURL(cmdBase + "services/instance?get")},
		{name: "services/plan/extra", req: withURL(cmdBase + "services/plan/extra?get")},
		{name: "no version", req: withURL("https://" + hierHost + "/command//services/plan?get")},
		{name: "GET", req: func(t *testing.T) *http.Request {
			r := hierCommand(t, lookupPlanURL, hierSession, valid)
			r.Method = http.MethodGet
			return r
		}},
		{name: "no session header", req: func(t *testing.T) *http.Request {
			return hierCommand(t, lookupPlanURL, "", valid)
		}},
		{name: "GetBody nil", req: func(t *testing.T) *http.Request {
			r := hierCommand(t, lookupPlanURL, hierSession, valid)
			r.GetBody = nil
			return r
		}},
		{name: "by name", req: withBody(`{"paramValues":{"name":"lookup-plan","offeringName":"lookup-offering","subaccount":"` + hierSubaccount + `"}}`)},
		{name: "extra param", req: withBody(`{"paramValues":{"id":"` + lookupPlanID + `","fieldsFilter":"x","subaccount":"` + hierSubaccount + `"}}`)},
		{name: "empty id", req: withBody(lookupBody(hierSubaccount, ""))},
		{name: "empty subaccount", req: withBody(lookupBody("", lookupPlanID))},
		{name: "id not a string", req: withBody(`{"paramValues":{"id":123,"subaccount":"` + hierSubaccount + `"}}`)},
		{name: "id null", req: withBody(`{"paramValues":{"id":null,"subaccount":"` + hierSubaccount + `"}}`)},
		{name: "paramValues not an object", req: withBody(`{"paramValues":"x"}`)},
		{name: "extra top-level key", req: withBody(`{"paramValues":{"id":"` + lookupPlanID + `","subaccount":"` + hierSubaccount + `"},"x":1}`)},
		{name: "invalid JSON", req: withBody(`{"paramValues":`)},
		{name: "empty body", req: withBody("")},
		{name: "body over 1 MiB", req: withBody(valid + strings.Repeat(" ", maxCommandBodyBytes))},
		{name: "nil cache", nilCache: true, req: withURL(lookupPlanURL)},
	}
	for _, tc := range nilCases {
		t.Run(tc.name, func(t *testing.T) {
			cache := c
			if tc.nilCache {
				cache = nil
			}
			if q := cache.parse(tc.req(t)); q != nil {
				t.Errorf("parse = %+v, want nil", q)
			}
		})
	}
}

type lookupKeySpec struct {
	host, version, command, subaccount, id, session, subdomain, idp, corr string
	swapped                                                               bool
}

func (s lookupKeySpec) request(t *testing.T) *http.Request {
	t.Helper()
	body := lookupBody(s.subaccount, s.id)
	if s.swapped {
		body = `{"paramValues":{"subaccount":"` + s.subaccount + `","id":"` + s.id + `"}}`
	}
	r := hierCommand(t, "https://"+s.host+"/command/"+s.version+"/"+s.command+"?get", s.session, body)
	r.Header.Set(hierWireSubdomain, s.subdomain)
	r.Header.Set(hierWireCustomIDP, s.idp)
	if s.corr != "" {
		r.Header.Set(hierWireCorrelationID, s.corr)
	}
	return r
}

func TestLookupCacheKey(t *testing.T) {
	c := newLookupCache(time.Hour, nil)
	base := lookupKeySpec{
		host: hierHost, version: "v2.106.1", command: "services/plan",
		subaccount: hierSubaccount, id: lookupPlanID, session: hierSession,
		subdomain: "lookup-ga", idp: "", corr: "corr-1",
	}
	key := func(t *testing.T, s lookupKeySpec) string {
		t.Helper()
		q := c.parse(s.request(t))
		if q == nil {
			t.Fatalf("parse(%+v) = nil", s)
		}
		return q.key
	}
	baseKey := key(t, base)

	t.Run("differs", func(t *testing.T) {
		variants := map[string]func(s *lookupKeySpec){
			"host":       func(s *lookupKeySpec) { s.host = "cli2.example.test" },
			"version":    func(s *lookupKeySpec) { s.version = "v2.107.0" },
			"command":    func(s *lookupKeySpec) { s.command = "services/offering" },
			"subaccount": func(s *lookupKeySpec) { s.subaccount = hierSubaccount2 },
			"id":         func(s *lookupKeySpec) { s.id = "cccccccc-dddd-4eee-8fff-000000000000" },
			"session":    func(s *lookupKeySpec) { s.session = "other-session" },
			"subdomain":  func(s *lookupKeySpec) { s.subdomain = "other-ga" },
			"idp":        func(s *lookupKeySpec) { s.idp = "idp.example.test" },
		}
		seen := map[string]string{baseKey: "base"}
		for name, mutate := range variants {
			s := base
			mutate(&s)
			k := key(t, s)
			if prev, dup := seen[k]; dup {
				t.Errorf("%s shares its key with %s", name, prev)
			}
			seen[k] = name
		}
	})

	t.Run("same", func(t *testing.T) {
		s := base
		s.corr = "corr-2"
		if key(t, s) != baseKey {
			t.Error("a different correlation id changed the key")
		}
		s = base
		s.swapped = true
		if key(t, s) != baseKey {
			t.Error("a different paramValues key order changed the key")
		}
	})

	t.Run("no clear values", func(t *testing.T) {
		for _, v := range []string{hierSession, hierSubaccount, lookupPlanID} {
			if strings.Contains(baseKey, v) {
				t.Errorf("key %q contains %q", baseKey, v)
			}
		}
	})
}

func TestLookupCacheHit(t *testing.T) {
	base := &hierBase{onCommand: lookupPlanResp}
	tr, _ := lookupTransport(base, nil)

	first, firstBody := lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, "corr-1"))
	hier, cmds := base.counts()
	if cmds != 1 || hier != 1 {
		t.Fatalf("after the first lookup: %d hierarchy calls, %d commands, want 1 and 1", hier, cmds)
	}
	if tr.lookups.len() != 1 {
		t.Fatalf("len() = %d, want 1", tr.lookups.len())
	}

	second, secondBody := lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, "corr-2"))
	if h, c := base.counts(); h != hier || c != cmds {
		t.Errorf("a hit reached the base: %d hierarchy calls, %d commands, want %d and %d", h, c, hier, cmds)
	}
	if firstBody != lookupPlanAnswer || secondBody != lookupPlanAnswer {
		t.Errorf("bodies = %q, %q, want %q", firstBody, secondBody, lookupPlanAnswer)
	}
	if first.StatusCode != 200 || second.StatusCode != 200 {
		t.Errorf("status = %d, %d, want 200", first.StatusCode, second.StatusCode)
	}
	if second.Header.Get(hierWireBackendStatus) != "200" {
		t.Errorf("backend status = %q, want 200", second.Header.Get(hierWireBackendStatus))
	}
	if second.Header.Get(lookupWireMediaType) != "application/json" {
		t.Errorf("backend media type = %q, want application/json", second.Header.Get(lookupWireMediaType))
	}
	if second.ContentLength != int64(len(lookupPlanAnswer)) {
		t.Errorf("ContentLength = %d, want %d", second.ContentLength, len(lookupPlanAnswer))
	}

	// The offering of the same id is another entry.
	lookupCall(t, tr, lookupReq(t, lookupOfferingURL, lookupPlanID, ""))
	if c := lookupCommands(t, base); c != cmds+1 {
		t.Errorf("offering lookup: %d commands, want %d", c, cmds+1)
	}
}

func TestLookupCacheReplayIsolation(t *testing.T) {
	base := &hierBase{onCommand: lookupPlanResp}
	tr, _ := lookupTransport(base, nil)
	lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, "corr-store"))

	a, err := tr.RoundTrip(lookupReq(t, lookupPlanURL, lookupPlanID, "corr-a"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := tr.RoundTrip(lookupReq(t, lookupPlanURL, lookupPlanID, "corr-b"))
	if err != nil {
		t.Fatal(err)
	}
	if lookupCommands(t, base) != 1 {
		t.Fatalf("hits reached the base: %d commands", lookupCommands(t, base))
	}
	if a == b {
		t.Fatal("two hits returned the same *http.Response")
	}

	aBody, _ := io.ReadAll(a.Body)
	bBody, _ := io.ReadAll(b.Body)
	if string(aBody) != lookupPlanAnswer || string(bBody) != lookupPlanAnswer {
		t.Errorf("bodies = %q, %q, want both complete", aBody, bBody)
	}

	if a.Header.Get(hierWireCorrelationID) != "corr-a" || b.Header.Get(hierWireCorrelationID) != "corr-b" {
		t.Errorf("correlation ids = %q, %q, want those of the current requests",
			a.Header.Get(hierWireCorrelationID), b.Header.Get(hierWireCorrelationID))
	}
	for _, h := range []string{hierWireSessionID, "Set-Cookie"} {
		if v := a.Header.Values(h); len(v) != 0 {
			t.Errorf("hit carries %s = %q", h, v)
		}
	}

	a.Header.Set(hierWireBackendStatus, "404")
	a.Header.Add(lookupWireMediaType, "text/plain")
	a.Header["Content-Type"][0] = "text/plain"
	c, cBody := lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, ""))
	for _, r := range []*http.Response{b, c} {
		if r.Header.Get(hierWireBackendStatus) != "200" ||
			len(r.Header.Values(lookupWireMediaType)) != 1 ||
			r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("header %v was changed through another hit", r.Header)
		}
	}
	if cBody != lookupPlanAnswer {
		t.Errorf("third body = %q", cBody)
	}
	if v := c.Header.Values(hierWireCorrelationID); len(v) != 0 {
		t.Errorf("hit without correlation id carries %q", v)
	}
}

func TestLookupCacheExpiry(t *testing.T) {
	base := &hierBase{onCommand: lookupPlanResp}
	tr, clk := lookupTransport(base, nil)
	lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, ""))

	clk.advance(time.Hour - time.Nanosecond)
	lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, ""))
	if c := lookupCommands(t, base); c != 1 {
		t.Fatalf("at ttl-1ns: %d commands, want 1 (hit)", c)
	}

	clk.advance(time.Nanosecond)
	_, body := lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, ""))
	if c := lookupCommands(t, base); c != 2 {
		t.Fatalf("at ttl: %d commands, want 2 (miss)", c)
	}
	if body != lookupPlanAnswer || tr.lookups.len() != 1 {
		t.Errorf("fresh answer: body %q, len %d, want it served and stored again", body, tr.lookups.len())
	}

	clk.advance(time.Hour - time.Nanosecond)
	lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, ""))
	if c := lookupCommands(t, base); c != 2 {
		t.Errorf("the fresh answer was not stored with a fresh time: %d commands, want 2", c)
	}
}

// lookupCloseCounter counts Close calls on a body.
type lookupCloseCounter struct {
	io.Reader
	mu     sync.Mutex
	closed int
}

func (b *lookupCloseCounter) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed++
	return nil
}

func (b *lookupCloseCounter) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

func TestLookupCacheNotStored(t *testing.T) {
	errBoom := errors.New("lookup boom")
	oversize := `"` + strings.Repeat("x", lookupCacheMaxBodyBytes-1) + `"`

	cases := []struct {
		name    string
		answer  func() (*http.Response, error)
		wantErr bool
		body    string
	}{
		{name: "backend 404", answer: func() (*http.Response, error) { return hierResp(200, "404", `{"error":"not found"}`), nil }, body: `{"error":"not found"}`},
		{name: "backend 500", answer: func() (*http.Response, error) { return hierResp(200, "500", `{"error":"x"}`), nil }, body: `{"error":"x"}`},
		{name: "no backend status", answer: func() (*http.Response, error) { return hierResp(200, "", `{"ok":true}`), nil }, body: `{"ok":true}`},
		{name: "http 503", answer: func() (*http.Response, error) { return hierResp(503, "200", `{"ok":true}`), nil }, body: `{"ok":true}`},
		{name: "bare 500", answer: failFastBare500, body: ""},
		{name: "transport error", answer: func() (*http.Response, error) { return nil, errBoom }, wantErr: true},
		{name: "body over the limit", answer: func() (*http.Response, error) { return hierResp(200, "200", oversize), nil }, body: oversize},
		{name: "invalid JSON", answer: func() (*http.Response, error) { return hierResp(200, "200", `not json`), nil }, body: "not json"},
		{name: "empty body", answer: func() (*http.Response, error) { return hierResp(200, "200", ""), nil }, body: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := &hierBase{onCommand: tc.answer}
			tr, _ := lookupTransport(base, nil)
			for i := 1; i <= 2; i++ {
				resp, err := tr.RoundTrip(lookupReq(t, lookupPlanURL, lookupPlanID, ""))
				if tc.wantErr {
					if !errors.Is(err, errBoom) {
						t.Fatalf("call %d: err = %v, want %v", i, err, errBoom)
					}
				} else {
					if err != nil {
						t.Fatalf("call %d: %v", i, err)
					}
					body, rerr := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if rerr != nil || string(body) != tc.body {
						t.Fatalf("call %d: body %d bytes, err %v, want the complete answer of %d bytes", i, len(body), rerr, len(tc.body))
					}
				}
				if n := tr.lookups.len(); n != 0 {
					t.Fatalf("call %d: len() = %d, want 0", i, n)
				}
			}
			if c := lookupCommands(t, base); c < 2 {
				t.Errorf("%d commands for 2 calls, want every call at the base", c)
			}
		})
	}

	t.Run("fail-fast replacement", func(t *testing.T) {
		base := &hierBase{onCommand: failFastBare500}
		tr, clk := failFastTransport(base, nil)
		tr.lookups = newLookupCache(time.Hour, nil)
		tr.lookups.now = clk.now
		resp, err := tr.RoundTrip(lookupReq(t, lookupPlanURL, lookupPlanID, ""))
		failFastIsReplacement(t, resp, err)
		if n := tr.lookups.len(); n != 0 {
			t.Fatalf("len() = %d after the replacement, want 0", n)
		}
		before := lookupCommands(t, base)
		resp, err = tr.RoundTrip(lookupReq(t, lookupPlanURL, lookupPlanID, ""))
		failFastIsReplacement(t, resp, err)
		if c := lookupCommands(t, base); c <= before {
			t.Errorf("the next lookup did not reach the base: %d commands, want more than %d", c, before)
		}
	})

	t.Run("read error", func(t *testing.T) {
		orig := &lookupCloseCounter{Reader: io.MultiReader(strings.NewReader(`{"par`), iotest.ErrReader(errBoom))}
		base := &hierBase{onCommand: func() (*http.Response, error) {
			resp := hierResp(200, "200", "")
			resp.Body = orig
			return resp, nil
		}}
		tr, _ := lookupTransport(base, nil)
		resp, err := tr.RoundTrip(lookupReq(t, lookupPlanURL, lookupPlanID, ""))
		if err != nil {
			t.Fatal(err)
		}
		body, rerr := io.ReadAll(resp.Body)
		if string(body) != `{"par` || !errors.Is(rerr, errBoom) {
			t.Errorf("caller read %q, %v, want the bytes read and %v", body, rerr, errBoom)
		}
		_ = resp.Body.Close()
		if orig.count() != 1 {
			t.Errorf("original body closed %d times, want 1", orig.count())
		}
		if n := tr.lookups.len(); n != 0 {
			t.Errorf("len() = %d, want 0", n)
		}
	})
}

func TestLookupCacheLimit(t *testing.T) {
	id := func(i int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", i) }

	t.Run("oldest goes", func(t *testing.T) {
		log := &hierLogger{}
		base := &hierBase{}
		tr, clk := lookupTransport(base, log)
		tr.lookups.maxEntries = 3
		for i := 1; i <= 4; i++ {
			lookupCall(t, tr, lookupReq(t, lookupPlanURL, id(i), ""))
			clk.advance(time.Second)
		}
		if n := tr.lookups.len(); n != 3 {
			t.Fatalf("len() = %d, want 3", n)
		}
		for i := 2; i <= 4; i++ {
			lookupCall(t, tr, lookupReq(t, lookupPlanURL, id(i), ""))
		}
		if c := lookupCommands(t, base); c != 4 {
			t.Fatalf("entries 2-4: %d commands, want 4 (all hits)", c)
		}
		lookupCall(t, tr, lookupReq(t, lookupPlanURL, id(1), ""))
		if c := lookupCommands(t, base); c != 5 {
			t.Fatalf("entry 1: %d commands, want 5 (dropped)", c)
		}
		lookupCall(t, tr, lookupReq(t, lookupPlanURL, id(5), ""))
		if n := tr.lookups.len(); n != 3 {
			t.Errorf("len() = %d, want 3", n)
		}
		lines := log.find("info", lookupFullMessage)
		if len(lines) != 1 {
			t.Fatalf("%d %q lines, want 1", len(lines), lookupFullMessage)
		}
		if kv := failFastKV(lines[0].kv); kv["maxEntries"] != 3 {
			t.Errorf("maxEntries = %v, want 3", kv["maxEntries"])
		}
	})

	t.Run("expired go first", func(t *testing.T) {
		log := &hierLogger{}
		base := &hierBase{}
		tr, clk := lookupTransport(base, log)
		tr.lookups.maxEntries = 3
		lookupCall(t, tr, lookupReq(t, lookupPlanURL, id(1), ""))
		clk.advance(50 * time.Minute)
		lookupCall(t, tr, lookupReq(t, lookupPlanURL, id(2), ""))
		lookupCall(t, tr, lookupReq(t, lookupPlanURL, id(3), ""))
		clk.advance(20 * time.Minute)
		lookupCall(t, tr, lookupReq(t, lookupPlanURL, id(4), ""))
		if n := tr.lookups.len(); n != 3 {
			t.Fatalf("len() = %d, want 3", n)
		}
		for i := 2; i <= 4; i++ {
			lookupCall(t, tr, lookupReq(t, lookupPlanURL, id(i), ""))
		}
		if c := lookupCommands(t, base); c != 4 {
			t.Errorf("live entries: %d commands, want 4 (all hits)", c)
		}
		if n := len(log.find("info", lookupFullMessage)); n != 0 {
			t.Errorf("%d %q lines, want 0 when only expired entries went", n, lookupFullMessage)
		}
	})

	t.Run("overwrite needs no room", func(t *testing.T) {
		base := &hierBase{}
		tr, _ := lookupTransport(base, nil)
		tr.lookups.maxEntries = 2
		lookupCall(t, tr, lookupReq(t, lookupPlanURL, id(1), ""))
		lookupCall(t, tr, lookupReq(t, lookupPlanURL, id(2), ""))
		// Two concurrent misses of one key both store their answer.
		tr.lookups.put(tr.lookups.parse(lookupReq(t, lookupPlanURL, id(2), "")), hierResp(200, "200", `{"ok":2}`))
		if n := tr.lookups.len(); n != 2 {
			t.Fatalf("len() = %d, want 2", n)
		}
		lookupCall(t, tr, lookupReq(t, lookupPlanURL, id(1), ""))
		if c := lookupCommands(t, base); c != 2 {
			t.Errorf("entry 1: %d commands, want 2 (hit)", c)
		}
		if _, body := lookupCall(t, tr, lookupReq(t, lookupPlanURL, id(2), "")); body != `{"ok":2}` {
			t.Errorf("entry 2 body = %q, want the overwritten answer", body)
		}
	})
}

func TestLookupCacheHitSkipsHierarchy(t *testing.T) {
	base := &hierBase{onCommand: lookupPlanResp}
	tr, clk := lookupTransport(base, nil)
	lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, ""))
	hier, cmds := base.counts()

	clk.advance(hierarchyCallTTL + time.Second)
	key := hierHost + "/" + hierSubaccount
	tr.hierarchy.markServed(key, true)
	clk.advance(time.Second)
	_, body := lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, ""))
	if h, c := base.counts(); h != hier || c != cmds {
		t.Errorf("hit after the hierarchy ttl: %d hierarchy calls, %d commands, want %d and %d", h, c, hier, cmds)
	}
	if body != lookupPlanAnswer {
		t.Errorf("body = %q", body)
	}
	if ago, _, _ := tr.hierarchy.guard(key, ""); ago != time.Second {
		t.Errorf("guard refreshed %v ago, want 1s: a hit counted as an answer", ago)
	}
}

func TestLookupCacheOff(t *testing.T) {
	if c := newLookupCache(0, nil); c != nil {
		t.Error("newLookupCache(0) != nil")
	}
	if c := newLookupCache(-time.Second, nil); c != nil {
		t.Error("newLookupCache(-1s) != nil")
	}
	base := &hierBase{onCommand: lookupPlanResp}
	tr, _ := hierTransport(base, nil)
	for i := 0; i < 2; i++ {
		lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, ""))
	}
	if c := lookupCommands(t, base); c != 2 {
		t.Errorf("%d commands for 2 lookups without a cache, want 2", c)
	}
}

func TestLookupCacheHitClosesRequestBody(t *testing.T) {
	base := &hierBase{onCommand: lookupPlanResp}
	tr, _ := lookupTransport(base, nil)
	lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, ""))

	req := lookupReq(t, lookupPlanURL, lookupPlanID, "")
	body := &lookupCloseCounter{Reader: strings.NewReader(lookupBody(hierSubaccount, lookupPlanID))}
	req.Body = body
	lookupCall(t, tr, req)
	if lookupCommands(t, base) != 1 {
		t.Fatal("second lookup was not a hit")
	}
	if body.count() != 1 {
		t.Errorf("request body closed %d times on a hit, want 1", body.count())
	}
}

func TestLookupCacheCancelledContext(t *testing.T) {
	base := &hierBase{onCommand: lookupPlanResp}
	tr, _ := lookupTransport(base, nil)
	lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, ""))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp, err := tr.RoundTrip(lookupReq(t, lookupPlanURL, lookupPlanID, "").WithContext(ctx))
	if err == nil {
		_ = resp.Body.Close()
	}
	if c := lookupCommands(t, base); c != 2 {
		t.Errorf("%d commands, want 2: a cancelled request must not be served from the cache", c)
	}
}

func TestLookupCacheLog(t *testing.T) {
	log := &hierLogger{}
	base := &hierBase{onCommand: lookupPlanResp}
	tr, clk := lookupTransport(base, log)
	lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, "corr-store"))
	clk.advance(1500 * time.Millisecond)
	lookupCall(t, tr, lookupReq(t, lookupPlanURL, lookupPlanID, "corr-hit"))

	lines := log.find("debug", lookupHitMessage)
	if len(lines) != 1 {
		t.Fatalf("%d %q lines, want 1", len(lines), lookupHitMessage)
	}
	kv := failFastKV(lines[0].kv)
	want := map[string]interface{}{
		"cliServerURL":  hierHost,
		"url":           hierHost + "/command/v2.106.1/services/plan",
		"subaccount":    hierSubaccount,
		"ageMs":         int64(1500),
		"correlationID": "corr-hit",
	}
	for k, v := range want {
		if kv[k] != v {
			t.Errorf("%s = %#v, want %#v", k, kv[k], v)
		}
	}
	key := tr.lookups.parse(lookupReq(t, lookupPlanURL, lookupPlanID, "")).key
	log.mu.Lock()
	defer log.mu.Unlock()
	for _, e := range log.entries {
		for _, v := range e.kv {
			if s := fmt.Sprint(v); strings.Contains(s, hierSession) || strings.Contains(s, key) {
				t.Errorf("%q logs %q", e.msg, s)
			}
		}
	}
}

func TestLookupCacheConcurrent(t *testing.T) {
	base := &hierBase{}
	tr, clk := lookupTransport(base, nil)
	tr.lookups.maxEntries = 5
	urls := []string{lookupPlanURL, lookupOfferingURL, hierCommandURL}

	var wg sync.WaitGroup
	errs := make(chan error, 32*40)
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				url := urls[(g+i)%len(urls)]
				id := fmt.Sprintf("00000000-0000-4000-8000-%012d", (g*i)%4)
				req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(lookupBody(hierSubaccount, id)))
				if err != nil {
					errs <- err
					return
				}
				req.Header.Set(hierWireFormat, "json")
				req.Header.Set(hierWireSessionID, hierSession)
				req.Header.Set(hierWireCorrelationID, fmt.Sprintf("corr-%d-%d", g, i))
				resp, err := tr.RoundTrip(req)
				if err != nil {
					errs <- err
					return
				}
				body, err := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil || string(body) != `{"ok":true}` {
					errs <- fmt.Errorf("body %q, err %v", body, err)
					return
				}
				if i%10 == 0 {
					clk.advance(time.Minute)
					_ = tr.lookups.len()
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if n := tr.lookups.len(); n > 5 {
		t.Errorf("len() = %d, want at most 5", n)
	}
}

func TestLookupCacheLeavesOtherCommands(t *testing.T) {
	base := &hierBase{}
	tr, _ := lookupTransport(base, nil)
	for i := 0; i < 3; i++ {
		lookupCall(t, tr, hierCommand(t, hierCommandURL, hierSession, lookupBody(hierSubaccount, lookupPlanID)))
	}
	if c := lookupCommands(t, base); c != 3 {
		t.Errorf("%d services/instance commands for 3 calls, want 3", c)
	}
	if n := tr.lookups.len(); n != 0 {
		t.Errorf("len() = %d, want 0", n)
	}
}
