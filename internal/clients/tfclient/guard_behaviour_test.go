package tfclient

// Behavioural tests for the write guard: a fake CLI server refuses subaccounts with
// a bare 500 on demand, and the tests check what the real terraform-provider-btp,
// the server and the log see. Time is injected, so nothing here waits.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

const (
	gapbSubA = "aaaaaaaa-1111-4222-8333-444444444444"
	gapbSubB = "bbbbbbbb-1111-4222-8333-444444444444"

	// Long and distinctive so that a leak into a log line cannot be a coincidence.
	gapbSession = "gapb-session-secret-placeholder"

	gapbHandUpMsg   = "cli server refused a subaccount after a write, leaving the retries to btpcli"
	gapbRecoveryMsg = "cli server served the subaccount after repeating the hierarchy call"
	gapbFailFastMsg = "cli server has not loaded the subaccount, failing the command without retries"
)

type gapbEvent struct {
	hierarchy bool
	path      string
	action    string
	header    http.Header
}

type gapbRefusal struct {
	left   int // negative refuses for good
	action string
	suffix string // command path suffix
}

// gapbServer is a CLI server whose behaviour per subaccount is scripted at run time.
type gapbServer struct {
	*httptest.Server

	mu       sync.Mutex
	events   map[string][]gapbEvent
	all      int
	refusals map[string]*gapbRefusal
	hierFail map[string]bool
	backend  map[string]string // subaccount + "|" + action
}

func gapbNewServer(t *testing.T) *gapbServer {
	t.Helper()
	s := &gapbServer{
		events:   map[string][]gapbEvent{},
		refusals: map[string]*gapbRefusal{},
		hierFail: map[string]bool{},
		backend:  map[string]string{},
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

func (s *gapbServer) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/login/"):
		w.Header().Set("X-Cpcli-Sessionid", behaviourSession)
		_, _ = w.Write([]byte(`{"mail":"u@example.test","issuer":"https://idp.example.test"}`))

	case strings.HasPrefix(r.URL.Path, "/client/") && strings.HasSuffix(r.URL.Path, "/globalAccountHierarchyForNodes"):
		var req struct {
			Nodes []struct {
				EntityGUID string `json:"entityGuid"`
			} `json:"nodes"`
		}
		_ = json.Unmarshal(body, &req)
		failed := false
		s.mu.Lock()
		for _, n := range req.Nodes {
			s.record(n.EntityGUID, gapbEvent{hierarchy: true, path: r.URL.Path, header: r.Header.Clone()})
			failed = failed || s.hierFail[n.EntityGUID]
		}
		s.mu.Unlock()
		if failed {
			w.Header().Set("X-Cpcli-Backend-Status", "500")
			_, _ = w.Write([]byte(`{"error":"hierarchy failed"}`))
			return
		}
		w.Header().Set("X-Cpcli-Backend-Status", "200")
		_, _ = w.Write([]byte(`{"guid":"` + behaviourGA + `"}`))

	case strings.Contains(r.URL.Path, "/command/"):
		var req struct {
			ParamValues struct {
				Subaccount string `json:"subaccount"`
			} `json:"paramValues"`
		}
		_ = json.Unmarshal(body, &req)
		sa, action := req.ParamValues.Subaccount, r.URL.RawQuery
		s.mu.Lock()
		s.record(sa, gapbEvent{path: r.URL.Path, action: action, header: r.Header.Clone()})
		refuse := false
		if ref := s.refusals[sa]; ref != nil && (ref.action == "" || ref.action == action) && strings.HasSuffix(r.URL.Path, ref.suffix) && ref.left != 0 {
			refuse = true
			if ref.left > 0 {
				ref.left--
			}
		}
		backend, ok := s.backend[sa+"|"+action]
		s.mu.Unlock()
		if refuse {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !ok {
			backend = "200"
			if action == "create" {
				// Makes upstream read the instance right after the create.
				backend = "202"
			}
		}
		w.Header().Set("X-Cpcli-Backend-Status", backend)
		if n, _ := strconv.Atoi(backend); n >= 400 {
			_, _ = w.Write([]byte(`{"error":"rejected by the backend"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"` + ffbInstanceID + `","name":"inst","ready":true,"subaccount_id":"` + sa +
			`","last_operation":{"state":"succeeded"},"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}`))

	default:
		http.NotFound(w, r)
	}
}

// record must be called with s.mu held.
func (s *gapbServer) record(sa string, e gapbEvent) {
	s.events[sa] = append(s.events[sa], e)
	s.all++
}

// refuseNext refuses n commands for sa (all of them when n is negative), only the
// ones with the given action and path suffix when they are not empty.
func (s *gapbServer) refuseNext(sa string, n int, action, suffix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refusals[sa] = &gapbRefusal{left: n, action: action, suffix: suffix}
}

func (s *gapbServer) refuseAll(sa string) { s.refuseNext(sa, -1, "", "") }

func (s *gapbServer) failHierarchy(sa string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hierFail[sa] = true
}

func (s *gapbServer) answerWith(sa, action, backend string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backend[sa+"|"+action] = backend
}

func (s *gapbServer) eventsOf(sa string) []gapbEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]gapbEvent(nil), s.events[sa]...)
}

func (s *gapbServer) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.all
}

// gapbShape writes hierarchy calls as H and commands as C.
func gapbShape(evs []gapbEvent) string {
	var b strings.Builder
	for _, e := range evs {
		if e.hierarchy {
			b.WriteByte('H')
		} else {
			b.WriteByte('C')
		}
	}
	return b.String()
}

// gapbCommands counts the commands with the given action ("" counts all).
func gapbCommands(evs []gapbEvent, action string) int {
	n := 0
	for _, e := range evs {
		if !e.hierarchy && (action == "" || e.action == action) {
			n++
		}
	}
	return n
}

// gapbSleeps records the waits, and how many server requests had happened at each,
// instead of sleeping.
type gapbSleeps struct {
	mu    sync.Mutex
	waits []time.Duration
	seen  []int
	count func() int
	err   error
}

func (s *gapbSleeps) sleep(_ context.Context, d time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waits = append(s.waits, d)
	if s.count != nil {
		s.seen = append(s.seen, s.count())
	}
	return s.err
}

func (s *gapbSleeps) got() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.waits...)
}

func (s *gapbSleeps) seenAt() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int(nil), s.seen...)
}

func gapbNewTransport(srv *gapbServer, log *hierLogger, failFast bool) (*cliTransport, *behaviourClock, *gapbSleeps) {
	tr := newCLITransport(&cachingProvider{entries: map[string]*cacheEntry{}}, http.DefaultTransport, nil, true, failFast)
	if log != nil {
		tr.log = log
	}
	clk := newBehaviourClock()
	sl := &gapbSleeps{count: srv.total}
	tr.hierarchy.now = clk.Now
	tr.hierarchy.sleep = sl.sleep
	return tr, clk, sl
}

// gapbRig drives the transport directly, like btpcli does.
type gapbRig struct {
	srv    *gapbServer
	tr     *cliTransport
	clock  *behaviourClock
	sleeps *gapbSleeps
	log    *hierLogger
}

func gapbNewRig(t *testing.T, failFast bool) *gapbRig {
	t.Helper()
	srv := gapbNewServer(t)
	log := &hierLogger{}
	tr, clk, sl := gapbNewTransport(srv, log, failFast)
	return &gapbRig{srv: srv, tr: tr, clock: clk, sleeps: sl, log: log}
}

func gapbRequest(t *testing.T, serverURL, sa, action string) *http.Request {
	t.Helper()
	body := `{"paramValues":{"id":"` + ffbInstanceID + `","parameters":"false","subaccount":"` + sa + `"}}`
	req, err := http.NewRequest(http.MethodPost, serverURL+"/command/"+behaviourVersion+"/services/instance?"+action, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	// Set would canonicalise the names; btpcli's spelling is what the server sees.
	for k, v := range map[string]string{
		"Content-Type":      "application/json",
		"X-Cpcli-Format":    "json",
		"X-Cpcli-Sessionid": gapbSession,
		"X-Cpcli-Subdomain": behaviourGA,
		"X-Cpcli-Customidp": "",
		"X-Correlationid":   behaviourCorrID,
	} {
		req.Header[k] = []string{v}
	}
	return req
}

func (g *gapbRig) send(t *testing.T, sa, action string) (*http.Response, []byte, error) {
	t.Helper()
	resp, err := g.tr.RoundTrip(gapbRequest(t, g.srv.URL, sa, action))
	if resp == nil {
		return nil, nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	return resp, body, err
}

// served sends a command the server answers and checks that it did.
func (g *gapbRig) served(t *testing.T, sa, action string) {
	t.Helper()
	resp, _, err := g.send(t, sa, action)
	if err != nil {
		t.Fatalf("%s: %v", action, err)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Cpcli-Backend-Status") == "" || resp.Header.Get("X-Cpcli-Backend-Status") == "500" {
		t.Fatalf("%s: status %d, backend %q, want an answered command", action, resp.StatusCode, resp.Header.Get("X-Cpcli-Backend-Status"))
	}
}

// arm ends with the guard up for sa and the remembered hierarchy call older than
// the ttl. The next command then calls the hierarchy first, so a refusal is not
// absorbed by the single resend for a stale call and the resends the tests count
// are the guard's own.
func (g *gapbRig) arm(t *testing.T, sa, writeAction string) {
	t.Helper()
	g.served(t, sa, writeAction)
	g.clock.Advance(80 * time.Second)
	g.served(t, sa, "get")
	g.clock.Advance(15 * time.Second)
}

func gapbExpectBare(t *testing.T, resp *http.Response, body []byte) {
	t.Helper()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want the bare 500 handed up", resp.StatusCode)
	}
	if len(body) != 0 {
		t.Errorf("body = %q, want empty", body)
	}
	for k := range resp.Header {
		if strings.HasPrefix(http.CanonicalHeaderKey(k), "X-Cpcli-") {
			t.Errorf("response header %s present, want none", k)
		}
	}
}

func gapbExpectReplaced(t *testing.T, resp *http.Response, body []byte, sa string) {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Cpcli-Backend-Status"); got != "500" {
		t.Errorf("backend status = %q, want 500", got)
	}
	var parsed struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	if !strings.Contains(parsed.Error, "has not loaded subaccount "+sa) {
		t.Errorf("error = %q, want the not-loaded cause for %s", parsed.Error, sa)
	}
}

func gapbLines(l *hierLogger, msg string) []map[string]interface{} {
	var out []map[string]interface{}
	for _, e := range l.find("info", msg) {
		out = append(out, failFastKV(e.kv))
	}
	return out
}

func gapbExpectLine(t *testing.T, l *hierLogger, msg string, want int) []map[string]interface{} {
	t.Helper()
	lines := gapbLines(l, msg)
	if len(lines) != want {
		t.Fatalf("%q logged %d times, want %d", msg, len(lines), want)
	}
	return lines
}

func gapbExpectWaits(t *testing.T, sl *gapbSleeps, want []time.Duration) {
	t.Helper()
	if got := sl.got(); !reflect.DeepEqual(got, want) {
		t.Errorf("waits = %v, want %v", got, want)
	}
}

func gapbCheckKV(t *testing.T, kv map[string]interface{}, want map[string]interface{}) {
	t.Helper()
	for k, w := range want {
		if got, ok := kv[k]; !ok || fmt.Sprint(got) != fmt.Sprint(w) {
			t.Errorf("log key %s = %v, want %v", k, got, w)
		}
	}
}

// gapbRun is the state after one guarded command that met a refusing server.
type gapbRun struct {
	rig  *gapbRig
	base int // server requests before that command
	resp *http.Response
	body []byte
}

// gapbHandUp refuses the subaccount for good after a served write.
func gapbHandUp(t *testing.T) gapbRun {
	t.Helper()
	g := gapbNewRig(t, true)
	g.arm(t, gapbSubA, "create")
	g.srv.refuseAll(gapbSubA)
	base := g.srv.total()
	resp, body, err := g.send(t, gapbSubA, "get")
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	return gapbRun{g, base, resp, body}
}

// gapbRecover refuses the next two commands after a served write.
func gapbRecover(t *testing.T) gapbRun {
	t.Helper()
	g := gapbNewRig(t, true)
	g.arm(t, gapbSubA, "create")
	g.srv.refuseNext(gapbSubA, 2, "", "")
	base := g.srv.total()
	resp, body, err := g.send(t, gapbSubA, "get")
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	return gapbRun{g, base, resp, body}
}

// gapbProvider wires the real terraform-provider-btp through the transport.
func gapbProvider(t *testing.T) (*ffbEnv, *gapbServer, *behaviourClock, *gapbSleeps) {
	t.Helper()
	srv := gapbNewServer(t)
	tr, clk, sl := gapbNewTransport(srv, nil, true)
	env := ffbProvider(t, &ffbServer{Server: srv.Server}, tr)
	return env, srv, clk, sl
}

func gapbNo500(t *testing.T, probe *behaviourProbe) {
	t.Helper()
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if len(probe.statuses) == 0 {
		t.Fatal("probe saw no responses")
	}
	for _, s := range probe.statuses {
		if s == http.StatusInternalServerError {
			t.Errorf("provider side saw a 500; statuses = %v", probe.statuses)
			return
		}
	}
}

func gapbCause(t *testing.T, diags []*tfprotov6.Diagnostic, sa string) {
	t.Helper()
	errs := ffbErrors(diags)
	if len(errs) != 1 {
		t.Fatalf("got %d error diagnostics, want 1: %v", len(errs), diags)
	}
	if !strings.Contains(errs[0].Detail, ffbCausePart+sa) {
		t.Errorf("detail = %q, want the not-loaded cause for %s", errs[0].Detail, sa)
	}
}

func TestGapBehaviourReadAfterReadFailsFast(t *testing.T) {
	t.Parallel()
	env, srv, clk, sl := gapbProvider(t)

	resp, err := env.readResource(gapbSubA)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	behaviourFailOnErrors(t, "first read", resp.Diagnostics)

	// A stale remembered call makes the next command call first, so the one
	// refused command is the only one the server sees.
	clk.Advance(hierarchyCallTTL + time.Second)
	srv.refuseAll(gapbSubA)
	before := len(srv.eventsOf(gapbSubA))

	resp, err = env.readResource(gapbSubA)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	gapbCause(t, resp.Diagnostics, gapbSubA)

	if resp.NewState == nil {
		t.Fatal("new state is nil")
	}
	v, err := resp.NewState.Unmarshal(env.schemas.ResourceSchemas[ffbResource].ValueType())
	if err != nil {
		t.Fatalf("unmarshal new state: %v", err)
	}
	if v.IsNull() {
		t.Error("state was removed, want it kept")
	}
	gapbNo500(t, env.probe)
	if got := sl.got(); len(got) != 0 {
		t.Errorf("waits = %v, want none: reads never start the guard", got)
	}
	if tail := gapbShape(srv.eventsOf(gapbSubA)[before:]); tail != "HC" {
		t.Errorf("server saw %q after the refusal was set, want one hierarchy call and one refused command (HC)", tail)
	}
}

func TestGapBehaviourCreatePollSurvivesRefusals(t *testing.T) {
	t.Parallel()
	for _, refusals := range []int{2, 3, 4} {
		t.Run(fmt.Sprintf("%d refusals", refusals), func(t *testing.T) {
			t.Parallel()
			env, srv, _, sl := gapbProvider(t)
			// Upstream looks the plan up before the create, unguarded; only the polls
			// of the instance are refused.
			srv.refuseNext(gapbSubA, refusals, "get", "/services/instance")

			resp := env.applyResourceChange(t, nil, map[string]any{
				"subaccount_id":  gapbSubA,
				"name":           "n",
				"serviceplan_id": "p",
				// Upstream waits timeout/100 before polling; keep that at zero.
				"timeouts": map[string]any{"create": "10s"},
			})

			behaviourFailOnErrors(t, "create", resp.Diagnostics)
			if id := env.stateID(t, resp.NewState); id != ffbInstanceID {
				t.Errorf("state id after create = %q, want %q", id, ffbInstanceID)
			}
			gapbNo500(t, env.probe)

			waits := sl.got()
			if len(waits) < 1 || len(waits) > refusals || len(waits) > len(guardedResendWaits) {
				t.Fatalf("waits = %v, want between 1 and %d of %v", waits, refusals, guardedResendWaits)
			}
			if want := guardedResendWaits[:len(waits)]; !reflect.DeepEqual(waits, want) {
				t.Errorf("waits = %v, want the prefix %v", waits, want)
			}

			evs := srv.eventsOf(gapbSubA)
			if gets := gapbCommands(evs, "get"); gets < refusals+1 {
				t.Errorf("server saw %d reads, want the %d refused ones and a served one", gets, refusals)
			}
			// Every wait is followed by a hierarchy call and a resend, so the
			// server saw at least that many after the first refused read.
			first := -1
			for i, e := range evs {
				if !e.hierarchy && e.action == "get" {
					first = i
					break
				}
			}
			if first < 0 {
				t.Fatal("server saw no read")
			}
			after := evs[first+1:]
			hier := len(after) - gapbCommands(after, "")
			if hier < len(waits) || gapbCommands(after, "get") < len(waits) {
				t.Errorf("after the first read the server saw %q, want a hierarchy call and a read per wait (%d)", gapbShape(after), len(waits))
			}
		})
	}
}

func TestGapBehaviourOtherSubaccountUnaffected(t *testing.T) {
	t.Parallel()
	env, srv, _, sl := gapbProvider(t)

	resp := env.applyResourceChange(t, nil, map[string]any{
		"subaccount_id":  gapbSubA,
		"name":           "n",
		"serviceplan_id": "p",
		"timeouts":       map[string]any{"create": "10s"},
	})
	behaviourFailOnErrors(t, "create", resp.Diagnostics)

	srv.refuseAll(gapbSubB)
	bresp, err := env.readResource(gapbSubB)
	if err != nil {
		t.Fatalf("read on B: %v", err)
	}
	gapbCause(t, bresp.Diagnostics, gapbSubB)
	if n := gapbCommands(srv.eventsOf(gapbSubB), ""); n != 1 {
		t.Errorf("server saw %d commands for B, want 1", n)
	}

	aresp, err := env.readResource(gapbSubA)
	if err != nil {
		t.Fatalf("read on A: %v", err)
	}
	behaviourFailOnErrors(t, "read on A", aresp.Diagnostics)
	if got := sl.got(); len(got) != 0 {
		t.Errorf("waits = %v, want none", got)
	}
}

func TestGapBehaviourHandUpAfterWrite(t *testing.T) {
	t.Parallel()
	run := gapbHandUp(t)
	g := run.rig

	gapbExpectBare(t, run.resp, run.body)
	gapbExpectWaits(t, g.sleeps, guardedResendWaits)
	tail := g.srv.eventsOf(gapbSubA)[len(g.srv.eventsOf(gapbSubA))-8:]
	if got := gapbShape(tail); got != "HCHCHCHC" {
		t.Errorf("server saw %q, want the refused command and three resends, each after a hierarchy call", got)
	}
	// The waits sit between the resends, not before the first attempt.
	want := []int{run.base + 2, run.base + 4, run.base + 6}
	if got := g.sleeps.seenAt(); !reflect.DeepEqual(got, want) {
		t.Errorf("server requests seen at each wait = %v, want %v", got, want)
	}

	lines := gapbExpectLine(t, g.log, gapbHandUpMsg, 1)
	gapbCheckKV(t, lines[0], map[string]interface{}{
		"cliServerURL":    strings.TrimPrefix(g.srv.URL, "http://"),
		"subaccount":      gapbSubA,
		"lastServedAgoMs": 15000,
		"resends":         3,
		"correlationID":   behaviourCorrID,
	})
	gapbExpectLine(t, g.log, gapbRecoveryMsg, 0)
	gapbExpectLine(t, g.log, gapbFailFastMsg, 0)
}

func TestGapBehaviourNoLoopOnUpstreamRetry(t *testing.T) {
	t.Parallel()
	run := gapbHandUp(t)
	g := run.rig
	before := g.srv.total()

	resp, body, err := g.send(t, gapbSubA, "get")
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}

	gapbExpectBare(t, resp, body)
	gapbExpectWaits(t, g.sleeps, guardedResendWaits)
	tail := g.srv.eventsOf(gapbSubA)
	if got := gapbShape(tail[len(tail)-(g.srv.total()-before):]); got != "HC" {
		t.Errorf("server saw %q for the repeated command, want one hierarchy call and one command (HC)", got)
	}
	lines := gapbExpectLine(t, g.log, gapbHandUpMsg, 2)
	gapbCheckKV(t, lines[1], map[string]interface{}{"resends": 0, "subaccount": gapbSubA})
}

func TestGapBehaviourGuardExpiresAndExtends(t *testing.T) {
	t.Parallel()
	t.Run("expires after the ttl", func(t *testing.T) {
		t.Parallel()
		g := gapbNewRig(t, true)
		g.served(t, gapbSubA, "create")
		g.clock.Advance(91 * time.Second)
		g.srv.refuseAll(gapbSubA)

		resp, body, err := g.send(t, gapbSubA, "get")
		if err != nil {
			t.Fatalf("round trip: %v", err)
		}
		gapbExpectReplaced(t, resp, body, gapbSubA)
		gapbExpectWaits(t, g.sleeps, nil)
		gapbExpectLine(t, g.log, gapbFailFastMsg, 1)
		gapbExpectLine(t, g.log, gapbHandUpMsg, 0)
	})

	t.Run("an answered read extends it", func(t *testing.T) {
		t.Parallel()
		g := gapbNewRig(t, true)
		g.served(t, gapbSubA, "create")
		g.clock.Advance(60 * time.Second)
		g.served(t, gapbSubA, "get")
		g.clock.Advance(60 * time.Second)
		g.srv.refuseAll(gapbSubA)

		resp, body, err := g.send(t, gapbSubA, "get")
		if err != nil {
			t.Fatalf("round trip: %v", err)
		}
		gapbExpectBare(t, resp, body)
		gapbExpectWaits(t, g.sleeps, guardedResendWaits)
		lines := gapbExpectLine(t, g.log, gapbHandUpMsg, 1)
		gapbCheckKV(t, lines[0], map[string]interface{}{"lastServedAgoMs": 60000})
	})

	t.Run("a read alone starts nothing", func(t *testing.T) {
		t.Parallel()
		g := gapbNewRig(t, true)
		g.served(t, gapbSubA, "get")
		g.clock.Advance(time.Second)
		g.srv.refuseAll(gapbSubA)

		resp, body, err := g.send(t, gapbSubA, "get")
		if err != nil {
			t.Fatalf("round trip: %v", err)
		}
		gapbExpectReplaced(t, resp, body, gapbSubA)
		gapbExpectWaits(t, g.sleeps, nil)
		gapbExpectLine(t, g.log, gapbHandUpMsg, 0)
	})
}

func TestGapBehaviourFailedWriteDoesNotGuard(t *testing.T) {
	t.Parallel()
	g := gapbNewRig(t, true)
	g.srv.answerWith(gapbSubA, "create", "400")
	g.srv.answerWith(gapbSubA, "update", "409")
	for _, action := range []string{"create", "update"} {
		resp, _, err := g.send(t, gapbSubA, action)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d, want the rejected write passed on", action, resp.StatusCode)
		}
	}
	g.srv.refuseAll(gapbSubA)

	resp, body, err := g.send(t, gapbSubA, "get")
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	gapbExpectReplaced(t, resp, body, gapbSubA)
	gapbExpectWaits(t, g.sleeps, nil)
	gapbExpectLine(t, g.log, gapbHandUpMsg, 0)
}

func TestGapBehaviourActions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		action  string
		guarded bool
	}{
		{"update", true},
		{"delete", true},
		{"share", true},
		{"unregister", true},
		{"get", false},
		{"list", false},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			t.Parallel()
			g := gapbNewRig(t, true)
			g.served(t, gapbSubA, tc.action)
			g.srv.refuseAll(gapbSubA)

			resp, body, err := g.send(t, gapbSubA, "get")
			if err != nil {
				t.Fatalf("round trip: %v", err)
			}
			if tc.guarded {
				gapbExpectBare(t, resp, body)
				gapbExpectWaits(t, g.sleeps, guardedResendWaits)
				return
			}
			gapbExpectReplaced(t, resp, body, gapbSubA)
			gapbExpectWaits(t, g.sleeps, nil)
		})
	}
}

func TestGapBehaviourCancelledWait(t *testing.T) {
	t.Parallel()
	g := gapbNewRig(t, true)
	g.arm(t, gapbSubA, "create")
	g.srv.refuseAll(gapbSubA)
	g.sleeps.err = context.Canceled
	base := len(g.srv.eventsOf(gapbSubA))

	resp, err := g.tr.RoundTrip(gapbRequest(t, g.srv.URL, gapbSubA, "get"))
	if resp != nil {
		_ = resp.Body.Close()
		t.Errorf("response = %v, want nil", resp.Status)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := gapbShape(g.srv.eventsOf(gapbSubA)[base:]); got != "HC" {
		t.Errorf("server saw %q, want the refused command only (HC)", got)
	}
	gapbExpectWaits(t, g.sleeps, guardedResendWaits[:1])
	gapbExpectLine(t, g.log, gapbHandUpMsg, 0)
}

func TestGapBehaviourHierarchyFailureStopsResends(t *testing.T) {
	t.Parallel()
	g := gapbNewRig(t, true)
	g.arm(t, gapbSubA, "create")
	g.srv.refuseAll(gapbSubA)
	g.srv.failHierarchy(gapbSubA)
	base := len(g.srv.eventsOf(gapbSubA))

	resp, body, err := g.send(t, gapbSubA, "get")
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}

	gapbExpectBare(t, resp, body)
	gapbExpectWaits(t, g.sleeps, guardedResendWaits[:1])
	// A resend to a subaccount that could not be loaded is refused again, so the
	// command is sent once and the failed call repeated once.
	if got := gapbShape(g.srv.eventsOf(gapbSubA)[base:]); got != "HCH" {
		t.Errorf("server saw %q, want a failed hierarchy call, the command and one failed repeat (HCH)", got)
	}
	lines := gapbExpectLine(t, g.log, gapbHandUpMsg, 1)
	gapbCheckKV(t, lines[0], map[string]interface{}{"resends": 0})
}

func TestGapBehaviourRecoveryLog(t *testing.T) {
	t.Parallel()
	run := gapbRecover(t)
	g := run.rig

	if run.resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", run.resp.StatusCode)
	}
	if got := run.resp.Header.Get("X-Cpcli-Backend-Status"); got != "200" {
		t.Errorf("backend status = %q, want 200", got)
	}
	if !strings.Contains(string(run.body), ffbInstanceID) {
		t.Errorf("body = %s, want the instance the server served", run.body)
	}
	gapbExpectWaits(t, g.sleeps, guardedResendWaits[:2])

	lines := gapbExpectLine(t, g.log, gapbRecoveryMsg, 1)
	gapbCheckKV(t, lines[0], map[string]interface{}{
		"cliServerURL":  strings.TrimPrefix(g.srv.URL, "http://"),
		"subaccount":    gapbSubA,
		"resends":       2,
		"correlationID": behaviourCorrID,
	})
	gapbExpectLine(t, g.log, gapbHandUpMsg, 0)
	gapbExpectLine(t, g.log, gapbFailFastMsg, 0)
}

func TestGapBehaviourSessionNeverLogged(t *testing.T) {
	t.Parallel()
	for name, run := range map[string]gapbRun{"hand-up": gapbHandUp(t), "recovery": gapbRecover(t)} {
		l := run.rig.log
		l.mu.Lock()
		if len(l.entries) == 0 {
			t.Errorf("%s: nothing was logged, so the check would prove nothing", name)
		}
		for _, e := range l.entries {
			if strings.Contains(e.msg, gapbSession) || strings.Contains(fmt.Sprint(e.kv...), gapbSession) {
				t.Errorf("%s: log line %q leaks the session id: %v", name, e.msg, e.kv)
			}
		}
		l.mu.Unlock()
	}
}

func TestGapBehaviourSwitchedOff(t *testing.T) {
	t.Parallel()
	g := gapbNewRig(t, false)
	g.served(t, gapbSubA, "create")
	g.srv.refuseAll(gapbSubA)

	resp, body, err := g.send(t, gapbSubA, "get")
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}

	gapbExpectBare(t, resp, body)
	gapbExpectWaits(t, g.sleeps, nil)
	gapbExpectLine(t, g.log, gapbHandUpMsg, 0)
	gapbExpectLine(t, g.log, gapbFailFastMsg, 0)
}
