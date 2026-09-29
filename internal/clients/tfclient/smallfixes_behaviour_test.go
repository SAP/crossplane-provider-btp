package tfclient

// Behavioural tests for the small fixes: the update path through the real
// terraform-provider-btp, the per-attempt request timeout and the pruning of
// idle subaccounts, all against fake CLI servers.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const sfbOtherSubaccount = "66666666-7777-4888-8999-aaaaaaaaaaaa"

// Upstream accepts an update only with backend status 202.
type sfbAccepted struct{ http.ResponseWriter }

func (w sfbAccepted) accept() {
	if w.Header().Get("X-Cpcli-Backend-Status") == "200" {
		w.Header().Set("X-Cpcli-Backend-Status", "202")
	}
}
func (w sfbAccepted) WriteHeader(code int) { w.accept(); w.ResponseWriter.WriteHeader(code) }
func (w sfbAccepted) Write(b []byte) (int, error) {
	w.accept()
	return w.ResponseWriter.Write(b)
}

func sfbNewServer(t *testing.T, flakyGets int) *ffbServer {
	t.Helper()
	s := &ffbServer{
		flakyGets:    flakyGets,
		hierarchy:    map[string]int{},
		commands:     map[string]int{},
		gets:         map[string]int{},
		commandsSeen: map[string][]http.Header{},
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery == "update" {
			w = sfbAccepted{w}
		}
		s.handle(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func sfbStates(sa string) (prior, planned map[string]any) {
	prior = map[string]any{"subaccount_id": sa, "id": ffbInstanceID, "name": "n", "serviceplan_id": "p"}
	planned = map[string]any{"subaccount_id": sa, "id": ffbInstanceID, "name": "n2", "serviceplan_id": "p",
		// Upstream waits timeout/100 before polling; keep that at zero.
		"timeouts": map[string]any{"update": "10s"}}
	return prior, planned
}

func TestSmallFixesBehaviourUpdateReadAfterServedWriteIsRetried(t *testing.T) {
	t.Parallel()
	// The refusals outlast the transport's own resend and its guarded resends, so
	// upstream has to retry.
	flakyGets := 2 + len(guardedResendWaits)
	srv := sfbNewServer(t, flakyGets)
	tr := ffbTransport(true, true)
	tr.hierarchy.sleep = func(context.Context, time.Duration) error { return nil }
	env := ffbProvider(t, srv, tr)

	prior, planned := sfbStates(ffbFlaky)
	resp := env.applyResourceChange(t, prior, planned)

	behaviourFailOnErrors(t, "update", resp.Diagnostics)
	if got := env.stateID(t, resp.NewState); got != ffbInstanceID {
		t.Errorf("state id = %q, want %q", got, ffbInstanceID)
	}
	if n := srv.instanceGets(ffbFlaky); n < flakyGets+1 {
		t.Errorf("instance reads = %d, want at least %d", n, flakyGets+1)
	}
	env.probe.mu.Lock()
	defer env.probe.mu.Unlock()
	saw500 := false
	for _, s := range env.probe.statuses {
		if s == http.StatusInternalServerError {
			saw500 = true
		}
	}
	if !saw500 {
		t.Errorf("provider side never saw a 500, the bare 500 was replaced; statuses = %v", env.probe.statuses)
	}
}

func TestSmallFixesBehaviourUpdateNeverServedFailsFast(t *testing.T) {
	t.Parallel()
	srv := sfbNewServer(t, 0)
	env := ffbProvider(t, srv, ffbTransport(true, true))

	prior, planned := sfbStates(ffbFailing)
	resp := env.applyResourceChange(t, prior, planned)

	if corr := ffbCause(t, resp.Diagnostics); corr == "" {
		t.Error("want a correlation id in the error")
	}
	if n := srv.commandCalls(ffbFailing); n != 1 {
		t.Errorf("commands = %d, want 1", n)
	}
	if n := srv.hierarchyCalls(ffbFailing); n != 1 {
		t.Errorf("hierarchy calls = %d, want 1", n)
	}
	if got := env.stateID(t, resp.NewState); got != ffbInstanceID {
		t.Errorf("state id after failed update = %q, want %q", got, ffbInstanceID)
	}
	env.probe.mu.Lock()
	defer env.probe.mu.Unlock()
	for _, s := range env.probe.statuses {
		if s == http.StatusInternalServerError {
			t.Errorf("provider side saw a 500; statuses = %v", env.probe.statuses)
		}
	}
}

// sfbHangServer answers one of the two endpoints normally and blocks the other
// until the request ends or the server is torn down.
type sfbHangServer struct {
	*httptest.Server
	release chan struct{}

	mu        sync.Mutex
	hierarchy int
	commands  int
}

func sfbNewHangServer(t *testing.T, hangHierarchy, hangCommand bool) *sfbHangServer {
	t.Helper()
	s := &sfbHangServer{release: make(chan struct{})}
	block := func(r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-s.release:
		}
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		isHierarchy := strings.Contains(r.URL.Path, "/client/") && strings.HasSuffix(r.URL.Path, "/globalAccountHierarchyForNodes")
		s.mu.Lock()
		if isHierarchy {
			s.hierarchy++
		} else {
			s.commands++
		}
		s.mu.Unlock()
		if (isHierarchy && hangHierarchy) || (!isHierarchy && hangCommand) {
			block(r)
			return
		}
		w.Header().Set("X-Cpcli-Backend-Status", "200")
		_, _ = w.Write([]byte(`{}`))
	}))
	// Cleanups run last in, first out: release the handlers, then close.
	t.Cleanup(s.Close)
	t.Cleanup(func() { close(s.release) })
	return s
}

func (s *sfbHangServer) calls() (hierarchy, commands int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hierarchy, s.commands
}

func sfbExpectTimeout(t *testing.T, err error, elapsed time.Duration) {
	t.Helper()
	if err == nil {
		t.Fatal("want a timeout error, got none")
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) || !urlErr.Timeout() {
		t.Errorf("err = %v, want a *url.Error timeout", err)
	}
	if elapsed >= 5*time.Second {
		t.Errorf("call took %v, want it cut by the client timeout", elapsed)
	}
}

func TestSmallFixesBehaviourTimeoutCoversHierarchyCall(t *testing.T) {
	srv := sfbNewHangServer(t, true, false)
	hc := newCLIHTTPClient(ffbTransport(true, true))
	hc.Timeout = 100 * time.Millisecond

	start := time.Now()
	resp, err := hc.Do(behaviourCommand(t, srv.URL, behaviourSession, ffbHealthy))
	elapsed := time.Since(start)
	if resp != nil {
		_ = resp.Body.Close()
	}

	sfbExpectTimeout(t, err, elapsed)
	if _, commands := srv.calls(); commands != 0 {
		t.Errorf("command endpoint reached %d times, want 0", commands)
	}
}

func TestSmallFixesBehaviourTimeoutCoversCommand(t *testing.T) {
	srv := sfbNewHangServer(t, false, true)
	hc := newCLIHTTPClient(ffbTransport(true, true))
	hc.Timeout = 100 * time.Millisecond

	start := time.Now()
	resp, err := hc.Do(behaviourCommand(t, srv.URL, behaviourSession, ffbHealthy))
	elapsed := time.Since(start)
	if resp != nil {
		_ = resp.Body.Close()
	}

	sfbExpectTimeout(t, err, elapsed)
	if hierarchy, _ := srv.calls(); hierarchy != 1 {
		t.Errorf("hierarchy calls = %d, want 1", hierarchy)
	}
}

func TestSmallFixesBehaviourTimeoutLeavesNormalRequests(t *testing.T) {
	srv := sfbNewHangServer(t, false, false)
	hc := newCLIHTTPClient(ffbTransport(true, true))
	if hc.Timeout != cliRequestTimeout {
		t.Fatalf("timeout = %v, want %v", hc.Timeout, cliRequestTimeout)
	}

	resp, err := hc.Do(behaviourCommand(t, srv.URL, behaviourSession, ffbHealthy))
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Cpcli-Backend-Status") != "200" {
		t.Errorf("status = %d, backend status = %q, want 200/200", resp.StatusCode, resp.Header.Get("X-Cpcli-Backend-Status"))
	}
	if string(body) != `{}` {
		t.Errorf("body = %q", body)
	}
}

func sfbSend(t *testing.T, tr *cliTransport, srv *behaviourCLIServer, subaccount string) {
	t.Helper()
	if got := behaviourDo(t, tr, behaviourCommand(t, srv.URL, behaviourSession, subaccount)); got != http.StatusOK {
		t.Fatalf("status for %s = %d, want 200", subaccount, got)
	}
}

func sfbKeys(tr *cliTransport) []string {
	tr.hierarchy.mu.Lock()
	defer tr.hierarchy.mu.Unlock()
	var keys []string
	for k := range tr.hierarchy.entries {
		keys = append(keys, k)
	}
	return keys
}

func TestSmallFixesBehaviourIdleSubaccountsAreForgotten(t *testing.T) {
	clock := newBehaviourClock()
	// The server forgets a subaccount long before the entries are pruned.
	srv := newBehaviourCLIServer(t, clock, 3*time.Minute)
	tr := behaviourTransport(clock, true)

	sfbSend(t, tr, srv, behaviourSubaccount)
	clock.Advance(hierarchyEntryMaxIdle + time.Minute)
	sfbSend(t, tr, srv, sfbOtherSubaccount)

	keys := sfbKeys(tr)
	if len(keys) != 1 || !strings.HasSuffix(keys[0], "/"+sfbOtherSubaccount) {
		t.Fatalf("entries = %v, want only the entry for %s", keys, sfbOtherSubaccount)
	}
	before, _, _ := srv.counts()

	sfbSend(t, tr, srv, behaviourSubaccount)

	if after, _, _ := srv.counts(); after != before+1 {
		t.Errorf("hierarchy calls = %d, want %d", after, before+1)
	}
	if keys := sfbKeys(tr); len(keys) != 2 {
		t.Errorf("entries = %v, want 2", keys)
	}
}

func TestSmallFixesBehaviourActiveSubaccountsAreKept(t *testing.T) {
	clock := newBehaviourClock()
	srv := newBehaviourCLIServer(t, clock, 3*time.Minute)
	tr := behaviourTransport(clock, true)

	sfbSend(t, tr, srv, behaviourSubaccount)
	clock.Advance(hierarchyEntryMaxIdle - time.Minute)
	sfbSend(t, tr, srv, behaviourSubaccount)
	clock.Advance(2 * time.Minute)
	sfbSend(t, tr, srv, sfbOtherSubaccount)

	if keys := sfbKeys(tr); len(keys) != 2 {
		t.Errorf("entries = %v, want both subaccounts", keys)
	}
}
