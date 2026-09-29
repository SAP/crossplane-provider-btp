package btp

// Behavioural tests for the debug client: credentials of the CLI server must
// not reach the log, while the traffic itself stays untouched. The tests replace
// the package-level logger and therefore do not run in parallel.

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
)

type dbgLogger struct {
	mu   sync.Mutex
	logs []string
}

func (l *dbgLogger) record(msg string, kv []any) {
	parts := []string{msg}
	for _, v := range kv {
		parts = append(parts, fmt.Sprint(v))
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, strings.Join(parts, " "))
}

func (l *dbgLogger) Info(msg string, kv ...any)  { l.record(msg, kv) }
func (l *dbgLogger) Debug(msg string, kv ...any) { l.record(msg, kv) }
func (l *dbgLogger) WithValues(kv ...any) logging.Logger {
	return l
}

func (l *dbgLogger) output() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.logs, "\n")
}

func dbgCapture(t *testing.T) *dbgLogger {
	t.Helper()
	prev := log
	l := &dbgLogger{}
	SetLogger(l)
	t.Cleanup(func() { SetLogger(prev) })
	return l
}

type dbgBase func(*http.Request) (*http.Response, error)

func (f dbgBase) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func dbgRawRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "https://cli.example.test/command/v2.106.1/services/instance?get", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	// Written directly so the keys keep their case.
	req.Header["x-cpcli-sessionid"] = []string{"lower-request-placeholder"}
	req.Header["x-id-token"] = []string{"lower-token-placeholder"}
	return req
}

func dbgRawBase() dbgBase {
	return func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"x-cpcli-sessionid": []string{"lower-response-placeholder"}},
			Body:       http.NoBody,
		}, nil
	}
}

func TestDebugBehaviourCredentialsNeverLogged(t *testing.T) {
	captured := dbgCapture(t)

	var mu sync.Mutex
	var seen http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		seen = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("X-Cpcli-Sessionid", "response-session-placeholder")
		w.Header().Set("X-Cpcli-Backend-Status", "200")
		w.Header().Set("X-Correlationid", "corr-placeholder")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/command/v2.106.1/services/instance?get",
		strings.NewReader(`{"paramValues":{"subaccount":"11111111-2222-4333-8444-555555555555"}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Cpcli-Sessionid", "request-session-placeholder")
	req.Header.Set("X-Id-Token", "id-token-placeholder")
	req.Header.Set("Authorization", "Bearer auth-placeholder")
	req.Header.Set("X-Cpcli-Subdomain", "ga-placeholder")
	req.Header.Set("X-Correlationid", "corr-placeholder")

	resp, err := DebugPrintHTTPClient().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	out := captured.output()
	for _, secret := range []string{"request-session-placeholder", "response-session-placeholder", "id-token-placeholder", "auth-placeholder"} {
		if strings.Contains(out, secret) {
			t.Errorf("log contains %q:\n%s", secret, out)
		}
	}
	for _, want := range []string{"<REDACTED>", "ga-placeholder", "corr-placeholder"} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q:\n%s", want, out)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for k, want := range map[string]string{
		"X-Cpcli-Sessionid": "request-session-placeholder",
		"X-Id-Token":        "id-token-placeholder",
		"Authorization":     "Bearer auth-placeholder",
	} {
		if got := seen.Get(k); got != want {
			t.Errorf("server saw %s = %q, want %q", k, got, want)
		}
	}
	if got := resp.Header.Get("X-Cpcli-Sessionid"); got != "response-session-placeholder" {
		t.Errorf("response session id = %q", got)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q", body)
	}
}

func TestDebugBehaviourNonCanonicalKeysAreRedacted(t *testing.T) {
	captured := dbgCapture(t)

	resp, err := (&RoundTripDebugger{base: dbgRawBase()}).RoundTrip(dbgRawRequest(t))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = resp.Body.Close()

	out := captured.output()
	for _, secret := range []string{"lower-request-placeholder", "lower-token-placeholder", "lower-response-placeholder"} {
		if strings.Contains(out, secret) {
			t.Errorf("log contains %q:\n%s", secret, out)
		}
	}
}

func TestDebugBehaviourHeadersUntouchedForCaller(t *testing.T) {
	dbgCapture(t)

	req, err := http.NewRequest(http.MethodPost, "https://cli.example.test/command/v2.106.1/services/instance?get", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Cpcli-Sessionid", "request-session-placeholder")
	base := dbgBase(func(*http.Request) (*http.Response, error) {
		h := http.Header{}
		h.Set("X-Cpcli-Sessionid", "response-session-placeholder")
		return &http.Response{StatusCode: http.StatusOK, Header: h, Body: http.NoBody}, nil
	})

	resp, err := (&RoundTripDebugger{base: base}).RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = resp.Body.Close()

	if got := req.Header.Get("X-Cpcli-Sessionid"); got != "request-session-placeholder" {
		t.Errorf("request session id = %q", got)
	}
	if got := resp.Header.Get("X-Cpcli-Sessionid"); got != "response-session-placeholder" {
		t.Errorf("response session id = %q", got)
	}
}
