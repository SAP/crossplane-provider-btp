package tfclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	failFastCorrelationID = "ff-corr-0123"
	failFastLogMessage    = "cli server has not loaded the subaccount, failing the command without retries"
	failFastRecentMessage = "cli server refused a subaccount after a write, leaving the retries to btpcli"
)

// failFastTransport returns a transport with the hierarchy call and fail-fast on.
func failFastTransport(base http.RoundTripper, log *hierLogger) (*cliTransport, *hierClock) {
	var tr *cliTransport
	var clk *hierClock
	if log == nil {
		tr, clk = hierTransport(base, nil)
	} else {
		tr, clk = hierTransport(base, log)
	}
	tr.failFast = true
	return tr, clk
}

func failFastCommand(t *testing.T, corr string) *http.Request {
	t.Helper()
	req := hierCommand(t, hierCommandURL, hierSession, hierBody(hierSubaccount))
	if corr != "" {
		req.Header.Set(hierWireCorrelationID, corr)
	}
	return req
}

func failFastBare500() (*http.Response, error) { return hierBare500(), nil }

// failFastIsReplacement asserts the replacement shape and returns its error text.
func failFastIsReplacement(t *testing.T, resp *http.Response, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("RoundTrip error = %v, want nil", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || resp.Header.Get(hierWireBackendStatus) != "500" {
		t.Fatalf("answer = %d backend %q body %q, want the replacement (200, backend 500)",
			resp.StatusCode, resp.Header.Get(hierWireBackendStatus), body)
	}
	var parsed map[string]string
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body %q is not a JSON object of strings: %v", body, err)
	}
	return parsed["error"]
}

func failFastKV(kv []interface{}) map[string]interface{} {
	m := map[string]interface{}{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[fmt.Sprint(kv[i])] = kv[i+1]
	}
	return m
}

func TestFailFastResponseShape(t *testing.T) {
	t.Run("with correlation id", func(t *testing.T) {
		base := &hierBase{onCommand: failFastBare500}
		tr, _ := failFastTransport(base, nil)
		req := failFastCommand(t, failFastCorrelationID)

		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip error = %v, want nil", err)
		}
		defer resp.Body.Close() //nolint:errcheck
		body, _ := io.ReadAll(resp.Body)

		if resp.StatusCode != 200 || resp.Status != "200 OK" {
			t.Errorf("status = %d %q, want 200 \"200 OK\"", resp.StatusCode, resp.Status)
		}
		for k, want := range map[string]string{
			"X-Cpcli-Backend-Status": "500",
			"Content-Type":           "application/json",
			"X-Correlationid":        failFastCorrelationID,
		} {
			if got := resp.Header.Get(k); got != want {
				t.Errorf("%s = %q, want %q", k, got, want)
			}
		}
		if len(resp.Header) != 3 {
			t.Errorf("headers = %v, want exactly three", resp.Header)
		}
		if resp.ContentLength != int64(len(body)) {
			t.Errorf("ContentLength = %d, body length %d", resp.ContentLength, len(body))
		}
		if resp.Request != req {
			t.Error("Request is not the caller's request")
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatalf("body %q is not JSON: %v", body, err)
		}
		if len(parsed) != 1 {
			t.Fatalf("body fields = %v, want only \"error\"", parsed)
		}
		text, ok := parsed["error"].(string)
		if !ok {
			t.Fatalf("error field = %#v, want a string", parsed["error"])
		}
		for _, want := range []string{
			"has not loaded subaccount " + hierSubaccount,
			"Correlation ID: " + failFastCorrelationID,
		} {
			if !strings.Contains(text, want) {
				t.Errorf("error text %q lacks %q", text, want)
			}
		}
		// Upstream matches on these; the replacement must not trigger any of them.
		for _, bad := range []string{
			"[Error: 30004/400]", "[Error: 11006/429]", "Command timed out", "couldn't find resource", "/409", hierSession,
		} {
			if strings.Contains(text, bad) {
				t.Errorf("error text %q contains %q", text, bad)
			}
		}
	})

	t.Run("without correlation id", func(t *testing.T) {
		base := &hierBase{onCommand: failFastBare500}
		tr, _ := failFastTransport(base, nil)
		resp, err := tr.RoundTrip(failFastCommand(t, ""))
		text := failFastIsReplacement(t, resp, err)
		if v, ok := resp.Header["X-Correlationid"]; ok {
			t.Errorf("X-Correlationid = %v, want absent", v)
		}
		if strings.Contains(text, "Correlation ID") {
			t.Errorf("error text %q mentions a correlation id", text)
		}
		if !strings.Contains(text, "has not loaded subaccount "+hierSubaccount) {
			t.Errorf("error text %q lacks the subaccount", text)
		}
	})
}

func TestFailFastPaths(t *testing.T) {
	cases := []struct {
		name      string
		hier      func() (*http.Response, error)
		warm      bool
		hierarchy int
		commands  int
	}{
		// The warm-up remembers a hierarchy call but gets no backend answer, so the
		// subaccount does not count as served.
		{name: "hierarchy call fails", hier: func() (*http.Response, error) {
			return hierResp(401, "", "unauthorized"), nil
		}, hierarchy: 1, commands: 1},
		{name: "hierarchy call does not help", hierarchy: 1, commands: 1},
		{name: "stale remembered call repeated and command resent", warm: true, hierarchy: 2, commands: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := &hierBase{onHierarchy: tc.hier}
			tr, clk := failFastTransport(base, nil)
			if tc.warm {
				base.setCommand(func() (*http.Response, error) {
					return &http.Response{StatusCode: 503, Header: http.Header{}, Body: http.NoBody}, nil
				})
				if status, _ := hierRoundTrip(t, tr, failFastCommand(t, failFastCorrelationID)); status != 503 {
					t.Fatalf("warm-up status = %d", status)
				}
				clk.advance(time.Second)
			}
			base.setCommand(failFastBare500)

			resp, err := tr.RoundTrip(failFastCommand(t, failFastCorrelationID))
			failFastIsReplacement(t, resp, err)
			if h, c := base.counts(); h != tc.hierarchy || c != tc.commands {
				t.Fatalf("hierarchy=%d commands=%d, want %d/%d", h, c, tc.hierarchy, tc.commands)
			}
		})
	}
}

func TestFailFastLeavesOtherAnswers(t *testing.T) {
	dialErr := errors.New("dial fail")
	cases := []struct {
		name   string
		resp   func() (*http.Response, error)
		status int
		body   string
		err    error
	}{
		{"500 with body", func() (*http.Response, error) { return hierResp(500, "", "server trouble"), nil }, 500, "server trouble", nil},
		{"500 with backend status", func() (*http.Response, error) { return hierResp(500, "500", ""), nil }, 500, "", nil},
		{"502", func() (*http.Response, error) {
			return &http.Response{StatusCode: 502, Header: http.Header{}, Body: http.NoBody}, nil
		}, 502, "", nil},
		{"503", func() (*http.Response, error) {
			return &http.Response{StatusCode: 503, Header: http.Header{}, Body: http.NoBody}, nil
		}, 503, "", nil},
		{"429", func() (*http.Response, error) { return hierResp(429, "", "slow down"), nil }, 429, "slow down", nil},
		{"200 with backend 404", func() (*http.Response, error) { return hierResp(200, "404", `{"error":"not found"}`), nil }, 200, `{"error":"not found"}`, nil},
		{"transport error", func() (*http.Response, error) { return nil, dialErr }, 0, "", dialErr},
		// The server sent something the transport lost; it is no bare 500.
		{"500 with unreadable body", func() (*http.Response, error) {
			return &http.Response{StatusCode: 500, Header: http.Header{"Content-Type": {"text/plain"}}, ContentLength: -1, Body: failFastErrBody{}}, nil
		}, 500, "", nil},
		{"500 with content length", func() (*http.Response, error) {
			return &http.Response{StatusCode: 500, Header: http.Header{}, ContentLength: 42, Body: http.NoBody}, nil
		}, 500, "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := &hierBase{onCommand: tc.resp}
			log := &hierLogger{}
			tr, _ := failFastTransport(base, log)

			resp, err := tr.RoundTrip(failFastCommand(t, failFastCorrelationID))
			if tc.err != nil {
				if !errors.Is(err, tc.err) || resp != nil {
					t.Fatalf("RoundTrip = %v, %v, want nil, %v", resp, err, tc.err)
				}
			} else {
				if err != nil {
					t.Fatalf("RoundTrip error = %v", err)
				}
				defer resp.Body.Close() //nolint:errcheck
				body, _ := io.ReadAll(resp.Body)
				if resp.StatusCode != tc.status || string(body) != tc.body {
					t.Fatalf("answer = %d %q, want %d %q unchanged", resp.StatusCode, body, tc.status, tc.body)
				}
			}
			if n := len(log.find("info", failFastLogMessage)); n != 0 {
				t.Fatalf("fail-fast log lines = %d, want 0", n)
			}
		})
	}
}

func TestFailFastScope(t *testing.T) {
	valid := hierBody(hierSubaccount)
	cases := []struct {
		name string
		req  func(t *testing.T) *http.Request
	}{
		{"login POST without session header", func(t *testing.T) *http.Request {
			return hierCommand(t, "https://"+hierHost+"/login/v2.106.1", "", valid)
		}},
		{"command without subaccount", func(t *testing.T) *http.Request {
			return hierCommand(t, hierCommandURL, hierSession, `{"paramValues":{"id":"abc"}}`)
		}},
		{"GET", func(t *testing.T) *http.Request {
			r := hierCommand(t, hierCommandURL, hierSession, valid)
			r.Method = http.MethodGet
			return r
		}},
		{"GetBody nil", func(t *testing.T) *http.Request {
			r := hierCommand(t, hierCommandURL, hierSession, valid)
			r.GetBody = nil
			return r
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := &hierBase{onCommand: failFastBare500}
			tr, _ := failFastTransport(base, nil)
			status, body := hierRoundTrip(t, tr, tc.req(t))
			if status != 500 || body != "" {
				t.Fatalf("answer = %d %q, want the bare 500", status, body)
			}
			if h, c := base.counts(); h != 0 || c != 1 {
				t.Fatalf("hierarchy=%d commands=%d, want 0/1", h, c)
			}
		})
	}
}

func TestFailFastOff(t *testing.T) {
	t.Run("fail-fast off", func(t *testing.T) {
		base := &hierBase{onCommand: failFastBare500}
		tr, _ := hierTransport(base, nil)
		status, body := hierRoundTrip(t, tr, failFastCommand(t, failFastCorrelationID))
		if status != 500 || body != "" {
			t.Fatalf("answer = %d %q, want the bare 500", status, body)
		}
		if h, c := base.counts(); h != 1 || c != 1 {
			t.Fatalf("hierarchy=%d commands=%d, want 1/1", h, c)
		}
	})
	t.Run("hierarchy call off", func(t *testing.T) {
		base := &hierBase{onCommand: failFastBare500}
		tr, _ := failFastTransport(base, nil)
		tr.hierarchy = nil
		status, body := hierRoundTrip(t, tr, failFastCommand(t, failFastCorrelationID))
		if status != 500 || body != "" {
			t.Fatalf("answer = %d %q, want the bare 500", status, body)
		}
		if h, c := base.counts(); h != 0 || c != 1 {
			t.Fatalf("hierarchy=%d commands=%d, want 0/1", h, c)
		}
	})
}

// The next reconcile must start with a fresh hierarchy call, not send, call and
// resend against a call the bare 500 has disproved.
func TestFailFastForgets(t *testing.T) {
	base := &hierBase{onCommand: failFastBare500}
	log := &hierLogger{}
	tr, clk := failFastTransport(base, log)

	resp, err := tr.RoundTrip(failFastCommand(t, failFastCorrelationID))
	failFastIsReplacement(t, resp, err)
	h0, c0 := base.counts()

	clk.advance(time.Second)
	resp, err = tr.RoundTrip(failFastCommand(t, failFastCorrelationID))
	failFastIsReplacement(t, resp, err)
	if h, c := base.counts(); h != h0+1 || c != c0+1 {
		t.Fatalf("next attempt: hierarchy=%d commands=%d, want %d/%d (call first, no resend)", h, c, h0+1, c0+1)
	}
	if n := len(log.find("info", "cli server had not loaded the subaccount, repeating hierarchy call")); n != 0 {
		t.Fatalf("repeat log lines = %d, want 0", n)
	}
}

type failFastErrBody struct{}

func (failFastErrBody) Read([]byte) (int, error) { return 0, context.Canceled }
func (failFastErrBody) Close() error             { return nil }

// A poll that follows a served write must reach btpcli's retry: failing it fast
// reports the write as failed although it went through.
func TestFailFastRecentlyServed(t *testing.T) {
	cases := []struct {
		name    string
		backend string
	}{
		{"write served 200", "200"},
		{"write served 202", "202"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := &hierBase{onCommand: func() (*http.Response, error) { return hierResp(200, tc.backend, `{}`), nil }}
			log := &hierLogger{}
			tr, clk := failFastTransport(base, log)
			var mu sync.Mutex
			var waits []time.Duration
			tr.hierarchy.sleep = func(_ context.Context, d time.Duration) error {
				mu.Lock()
				defer mu.Unlock()
				waits = append(waits, d)
				return nil
			}
			write := failFastCommand(t, failFastCorrelationID)
			write.URL.RawQuery = "create"
			if status, _ := hierRoundTrip(t, tr, write); status != 200 {
				t.Fatalf("served answer status = %d", status)
			}
			clk.advance(time.Second)
			base.setCommand(failFastBare500)

			status, body := hierRoundTrip(t, tr, failFastCommand(t, failFastCorrelationID))
			if status != 500 || body != "" {
				t.Fatalf("answer within the ttl = %d %q, want the bare 500 handed up", status, body)
			}
			if n := len(log.find("info", failFastLogMessage)); n != 0 {
				t.Fatalf("fail-fast log lines = %d, want 0", n)
			}
			lines := log.find("info", failFastRecentMessage)
			if len(lines) != 1 {
				t.Fatalf("hand-up log lines = %d, want 1", len(lines))
			}
			kv := failFastKV(lines[0].kv)
			for k, want := range map[string]string{
				"subaccount":      hierSubaccount,
				"correlationID":   failFastCorrelationID,
				"cliServerURL":    hierHost,
				"lastServedAgoMs": "1000",
				"resends":         "3",
			} {
				if got := fmt.Sprint(kv[k]); got != want {
					t.Errorf("%s = %q, want %q", k, got, want)
				}
			}
			mu.Lock()
			if want := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}; fmt.Sprint(waits) != fmt.Sprint(want) {
				t.Errorf("waits = %v, want %v", waits, want)
			}
			mu.Unlock()

			// The handed-up bare 500 must not count as served. btpcli gives every
			// command its own correlation id; a reused one would be a retry.
			clk.advance(hierarchyCallTTL)
			resp, err := tr.RoundTrip(failFastCommand(t, "ff-corr-4567"))
			failFastIsReplacement(t, resp, err)
		})
	}
}

type failFastCountingBody struct {
	io.Reader
	mu     sync.Mutex
	closes int
}

func (b *failFastCountingBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closes++
	return nil
}

func TestFailFastClosesBareBody(t *testing.T) {
	body := &failFastCountingBody{Reader: strings.NewReader("")}
	base := &hierBase{onCommand: func() (*http.Response, error) {
		return &http.Response{StatusCode: 500, Header: http.Header{}, Body: body}, nil
	}}
	tr, _ := failFastTransport(base, nil)

	resp, err := tr.RoundTrip(failFastCommand(t, failFastCorrelationID))
	failFastIsReplacement(t, resp, err)
	if _, c := base.counts(); c != 1 {
		t.Fatalf("commands = %d, want 1", c)
	}
	body.mu.Lock()
	defer body.mu.Unlock()
	if body.closes != 1 {
		t.Fatalf("bare body closed %d times, want 1", body.closes)
	}
}

func TestFailFastLog(t *testing.T) {
	base := &hierBase{onCommand: failFastBare500}
	log := &hierLogger{}
	tr, _ := failFastTransport(base, log)

	resp, err := tr.RoundTrip(failFastCommand(t, failFastCorrelationID))
	failFastIsReplacement(t, resp, err)

	lines := log.find("info", failFastLogMessage)
	if len(lines) != 1 {
		t.Fatalf("fail-fast log lines = %d, want 1", len(lines))
	}
	kv := failFastKV(lines[0].kv)
	for k, want := range map[string]string{
		"subaccount":    hierSubaccount,
		"correlationID": failFastCorrelationID,
		"cliServerURL":  hierHost,
		"url":           hierHost + "/command/v2.106.1/services/instance",
	} {
		if got := fmt.Sprint(kv[k]); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	for _, e := range log.entries {
		if strings.Contains(e.msg, hierSession) || strings.Contains(fmt.Sprint(e.kv...), hierSession) {
			t.Fatalf("%s line %q leaks the session id: %v", e.level, e.msg, e.kv)
		}
	}
}

func TestFailFastSwitch(t *testing.T) {
	cp := &cachingProvider{entries: map[string]*cacheEntry{}}
	if tr := newCLITransport(cp, &hierBase{}, nil, true, true); !tr.failFast {
		t.Fatal("newCLITransport(..., true): failFast = false")
	}
	if tr := newCLITransport(cp, &hierBase{}, nil, true, false); tr.failFast {
		t.Fatal("newCLITransport(..., false): failFast = true")
	}

	old := failFastEnabled
	t.Cleanup(func() { SetFailFastOnUnloadedSubaccount(old) })
	SetFailFastOnUnloadedSubaccount(false)
	if failFastEnabled {
		t.Fatal("SetFailFastOnUnloadedSubaccount(false) did not switch off")
	}
	SetFailFastOnUnloadedSubaccount(true)
	if !failFastEnabled {
		t.Fatal("SetFailFastOnUnloadedSubaccount(true) did not switch on")
	}
}
