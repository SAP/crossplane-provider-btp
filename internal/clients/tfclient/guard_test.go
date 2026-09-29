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
	"time"
)

const guardRecoveryMessage = "cli server served the subaccount after repeating the hierarchy call"

// guardSleeps records the waits instead of sleeping.
type guardSleeps struct {
	mu    sync.Mutex
	waits []time.Duration
	err   error
}

func (s *guardSleeps) sleep(_ context.Context, d time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waits = append(s.waits, d)
	return s.err
}

func (s *guardSleeps) got() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.waits...)
}

func guardTransport(t *testing.T) (*cliTransport, *hierClock, *hierBase, *guardSleeps, *hierLogger) {
	t.Helper()
	base := &hierBase{}
	log := &hierLogger{}
	tr, clk := failFastTransport(base, log)
	sl := &guardSleeps{}
	tr.hierarchy.sleep = sl.sleep
	return tr, clk, base, sl, log
}

func guardRequest(t *testing.T, subaccount, action string) *http.Request {
	t.Helper()
	req := hierCommand(t, "https://"+hierHost+"/command/v2.106.1/services/instance?"+action, hierSession, hierBody(subaccount))
	req.Header.Set(hierWireCorrelationID, failFastCorrelationID)
	return req
}

// guardAnswer sends one command the base answers with HTTP 200 and backend.
func guardAnswer(t *testing.T, tr *cliTransport, base *hierBase, subaccount, action, backend string) {
	t.Helper()
	base.setCommand(func() (*http.Response, error) { return hierResp(200, backend, `{}`), nil })
	if status, _ := hierRoundTrip(t, tr, guardRequest(t, subaccount, action)); status != 200 {
		t.Fatalf("%s answer status = %d, want 200", action, status)
	}
}

// guardRefuse answers the first n commands with a bare 500 and all later ones.
func guardRefuse(n int) func() (*http.Response, error) {
	var mu sync.Mutex
	return func() (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		if n > 0 {
			n--
			return hierBare500(), nil
		}
		return hierResp(200, "200", `{"ok":true}`), nil
	}
}

func guardHandedUp(t *testing.T, tr *cliTransport, req *http.Request) {
	t.Helper()
	status, body := hierRoundTrip(t, tr, req)
	if status != 500 || body != "" {
		t.Fatalf("answer = %d %q, want the bare 500 handed up", status, body)
	}
}

func guardLine(t *testing.T, log *hierLogger, msg string, n int) map[string]interface{} {
	t.Helper()
	lines := log.find("info", msg)
	if len(lines) != n {
		t.Fatalf("%q lines = %d, want %d", msg, len(lines), n)
	}
	if n == 0 {
		return nil
	}
	return failFastKV(lines[n-1].kv)
}

func guardCheckKV(t *testing.T, kv map[string]interface{}, want map[string]string) {
	t.Helper()
	for k, w := range want {
		if got := fmt.Sprint(kv[k]); got != w {
			t.Errorf("%s = %q, want %q", k, got, w)
		}
	}
}

func TestGuardIsReadAction(t *testing.T) {
	cases := map[string]bool{
		"get": true, "list": true,
		"create": false, "update": false, "delete": false, "share": false, "unshare": false,
		"register": false, "unregister": false, "assign": false, "": false, "frobnicate": false,
	}
	l := newHierarchyLoader()
	for action, read := range cases {
		if got := isReadAction(action); got != read {
			t.Errorf("isReadAction(%q) = %v, want %v", action, got, read)
		}
		cmd := l.parse(guardRequest(t, hierSubaccount, action))
		if cmd == nil {
			t.Fatalf("parse(%q) = nil", action)
		}
		if cmd.write != !read {
			t.Errorf("parse(%q).write = %v, want %v", action, cmd.write, !read)
		}
	}
}

func TestGuardReadAfterReadFailsFast(t *testing.T) {
	tr, clk, base, sl, log := guardTransport(t)
	guardAnswer(t, tr, base, hierSubaccount, "get", "200")
	guardAnswer(t, tr, base, hierSubaccount, "list", "200")
	clk.advance(time.Second)
	base.setCommand(failFastBare500)

	resp, err := tr.RoundTrip(guardRequest(t, hierSubaccount, "get"))
	failFastIsReplacement(t, resp, err)
	if w := sl.got(); len(w) != 0 {
		t.Errorf("waits = %v, want none", w)
	}
	guardLine(t, log, failFastRecentMessage, 0)
	guardLine(t, log, failFastLogMessage, 1)
}

func TestGuardWriteStatus(t *testing.T) {
	for _, backend := range []string{"400", "404", "409", "500"} {
		t.Run("rejected "+backend, func(t *testing.T) {
			tr, clk, base, sl, log := guardTransport(t)
			guardAnswer(t, tr, base, hierSubaccount, "create", backend)
			clk.advance(time.Second)
			base.setCommand(failFastBare500)

			resp, err := tr.RoundTrip(guardRequest(t, hierSubaccount, "get"))
			failFastIsReplacement(t, resp, err)
			if w := sl.got(); len(w) != 0 {
				t.Errorf("waits = %v, want none", w)
			}
			guardLine(t, log, failFastRecentMessage, 0)
		})
	}
	t.Run("unparsable", func(t *testing.T) {
		tr, clk, base, sl, log := guardTransport(t)
		guardAnswer(t, tr, base, hierSubaccount, "create", "accepted")
		clk.advance(time.Second)
		base.setCommand(failFastBare500)

		guardHandedUp(t, tr, guardRequest(t, hierSubaccount, "get"))
		if w := sl.got(); len(w) != len(guardedResendWaits) {
			t.Errorf("waits = %v, want %v", w, guardedResendWaits)
		}
		guardLine(t, log, failFastRecentMessage, 1)
	})
}

func TestGuardExtension(t *testing.T) {
	t.Run("read inside the guard extends it", func(t *testing.T) {
		tr, clk, base, _, log := guardTransport(t)
		guardAnswer(t, tr, base, hierSubaccount, "create", "202")
		clk.advance(60 * time.Second)
		guardAnswer(t, tr, base, hierSubaccount, "get", "200")
		clk.advance(60 * time.Second)
		base.setCommand(failFastBare500)

		guardHandedUp(t, tr, guardRequest(t, hierSubaccount, "get"))
		kv := guardLine(t, log, failFastRecentMessage, 1)
		guardCheckKV(t, kv, map[string]string{"lastServedAgoMs": "60000", "resends": "3"})
	})
	t.Run("read after the guard does not revive it", func(t *testing.T) {
		tr, clk, base, sl, _ := guardTransport(t)
		guardAnswer(t, tr, base, hierSubaccount, "create", "202")
		clk.advance(91 * time.Second)
		guardAnswer(t, tr, base, hierSubaccount, "get", "200")
		clk.advance(time.Second)
		base.setCommand(failFastBare500)

		resp, err := tr.RoundTrip(guardRequest(t, hierSubaccount, "get"))
		failFastIsReplacement(t, resp, err)
		if w := sl.got(); len(w) != 0 {
			t.Errorf("waits = %v, want none", w)
		}
	})
}

func TestGuardResendRecovers(t *testing.T) {
	for k := 1; k <= len(guardedResendWaits); k++ {
		t.Run(fmt.Sprintf("%d resends", k), func(t *testing.T) {
			tr, clk, base, sl, log := guardTransport(t)
			guardAnswer(t, tr, base, hierSubaccount, "create", "200")
			h0, c0 := base.counts()
			clk.advance(time.Second)
			// sendCommand's send and resend, then k-1 refused guarded resends.
			base.setCommand(guardRefuse(k + 1))

			resp, err := tr.RoundTrip(guardRequest(t, hierSubaccount, "get"))
			if err != nil {
				t.Fatalf("RoundTrip error = %v", err)
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != 200 || resp.Header.Get(hierWireBackendStatus) != "200" || string(body) != `{"ok":true}` {
				t.Fatalf("answer = %d backend %q body %q, want the served answer",
					resp.StatusCode, resp.Header.Get(hierWireBackendStatus), body)
			}
			if w, want := sl.got(), guardedResendWaits[:k]; fmt.Sprint(w) != fmt.Sprint(want) {
				t.Errorf("waits = %v, want %v", w, want)
			}
			if h, c := base.counts(); h != h0+1+k || c != c0+2+k {
				t.Errorf("hierarchy=%d commands=%d, want %d/%d", h-h0, c-c0, 1+k, 2+k)
			}
			kv := guardLine(t, log, guardRecoveryMessage, 1)
			guardCheckKV(t, kv, map[string]string{
				"subaccount":    hierSubaccount,
				"correlationID": failFastCorrelationID,
				"cliServerURL":  hierHost,
				"resends":       fmt.Sprint(k),
			})
			guardLine(t, log, failFastRecentMessage, 0)
			key := hierHost + "/" + hierSubaccount
			if ago, guarded, handedUp := tr.hierarchy.guard(key, failFastCorrelationID); ago != 0 || !guarded || handedUp {
				t.Errorf("guard = %v %v %v, want refreshed now, up, not handed up", ago, guarded, handedUp)
			}
		})
	}
}

// guardExhaust runs one guarded bare 500 through all resends and the hand-up.
func guardExhaust(t *testing.T) (*cliTransport, *hierClock, *hierBase, *guardSleeps, *hierLogger) {
	t.Helper()
	tr, clk, base, sl, log := guardTransport(t)
	guardAnswer(t, tr, base, hierSubaccount, "create", "200")
	clk.advance(time.Second)
	base.setCommand(failFastBare500)
	guardHandedUp(t, tr, guardRequest(t, hierSubaccount, "get"))
	if h, c := base.counts(); h != 1+1+len(guardedResendWaits) || c != 1+2+len(guardedResendWaits) {
		t.Fatalf("hierarchy=%d commands=%d after the first hand-up", h, c)
	}
	kv := guardLine(t, log, failFastRecentMessage, 1)
	guardCheckKV(t, kv, map[string]string{"resends": fmt.Sprint(len(guardedResendWaits))})
	return tr, clk, base, sl, log
}

func TestGuardHandUpOnce(t *testing.T) {
	tr, clk, base, sl, log := guardExhaust(t)
	h0, c0 := base.counts()
	clk.advance(time.Second)

	guardHandedUp(t, tr, guardRequest(t, hierSubaccount, "get"))
	if w := sl.got(); len(w) != len(guardedResendWaits) {
		t.Errorf("waits = %v, want no more than the first loop's", w)
	}
	if h, c := base.counts(); h != h0+1 || c != c0+1 {
		t.Errorf("hierarchy=%d commands=%d, want +1/+1", h-h0, c-c0)
	}
	kv := guardLine(t, log, failFastRecentMessage, 2)
	guardCheckKV(t, kv, map[string]string{
		"subaccount":      hierSubaccount,
		"correlationID":   failFastCorrelationID,
		"cliServerURL":    hierHost,
		"lastServedAgoMs": "2000",
		"resends":         "0",
	})
	guardLine(t, log, failFastLogMessage, 0)
}

func TestGuardHandedUpClearedByAnswer(t *testing.T) {
	tr, clk, base, sl, log := guardExhaust(t)
	clk.advance(time.Second)
	guardAnswer(t, tr, base, hierSubaccount, "get", "200")
	clk.advance(time.Second)
	base.setCommand(failFastBare500)

	guardHandedUp(t, tr, guardRequest(t, hierSubaccount, "get"))
	if w := sl.got(); len(w) != 2*len(guardedResendWaits) {
		t.Errorf("waits = %v, want a second full loop", w)
	}
	kv := guardLine(t, log, failFastRecentMessage, 2)
	guardCheckKV(t, kv, map[string]string{"resends": fmt.Sprint(len(guardedResendWaits)), "lastServedAgoMs": "1000"})
}

func guardCorrRequest(t *testing.T, corr string) *http.Request {
	t.Helper()
	req := guardRequest(t, hierSubaccount, "get")
	req.Header.Set(hierWireCorrelationID, corr)
	return req
}

// guardSequence gives the answers in order and repeats the last one.
func guardSequence(answers ...func() (*http.Response, error)) func() (*http.Response, error) {
	var mu sync.Mutex
	return func() (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		a := answers[0]
		if len(answers) > 1 {
			answers = answers[1:]
		}
		return a()
	}
}

func TestGuardHandedUpRetryOutlivesGuard(t *testing.T) {
	tr, clk, _, sl, log := guardExhaust(t)
	// A refusal does not extend the guard, so btpcli's later retries run past it.
	clk.advance(95 * time.Second)

	guardHandedUp(t, tr, guardRequest(t, hierSubaccount, "get"))
	if w := sl.got(); len(w) != len(guardedResendWaits) {
		t.Errorf("waits = %v, want no more than the first loop's", w)
	}
	kv := guardLine(t, log, failFastRecentMessage, 2)
	guardCheckKV(t, kv, map[string]string{"lastServedAgoMs": "96000", "resends": "0"})
	guardLine(t, log, failFastLogMessage, 0)

	resp, err := tr.RoundTrip(guardCorrRequest(t, "other-corr"))
	failFastIsReplacement(t, resp, err)
	guardLine(t, log, failFastLogMessage, 1)
}

func TestGuardHandedUpPerCommand(t *testing.T) {
	tr, clk, _, sl, log := guardExhaust(t)
	clk.advance(time.Second)

	guardHandedUp(t, tr, guardCorrRequest(t, "other-corr"))
	if w := sl.got(); len(w) != 2*len(guardedResendWaits) {
		t.Errorf("waits = %v, want a second full loop", w)
	}
	kv := guardLine(t, log, failFastRecentMessage, 2)
	guardCheckKV(t, kv, map[string]string{
		"correlationID": "other-corr",
		"resends":       fmt.Sprint(len(guardedResendWaits)),
	})
}

func TestGuardUnansweredResend(t *testing.T) {
	cases := map[string]func() (*http.Response, error){
		"http 502":        func() (*http.Response, error) { return hierResp(502, "", "bad gateway"), nil },
		"transport error": func() (*http.Response, error) { return nil, errors.New("connection reset") },
	}
	for name, unanswered := range cases {
		t.Run(name, func(t *testing.T) {
			tr, clk, base, sl, log := guardTransport(t)
			guardAnswer(t, tr, base, hierSubaccount, "create", "200")
			clk.advance(time.Second)
			// sendCommand's send and resend, then the first guarded resend.
			base.setCommand(guardSequence(failFastBare500, failFastBare500, unanswered, failFastBare500))

			resp, err := tr.RoundTrip(guardRequest(t, hierSubaccount, "get"))
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode != 502 {
					t.Fatalf("answer = %d, want the 502 unchanged", resp.StatusCode)
				}
			}
			guardLine(t, log, guardRecoveryMessage, 0)
			guardLine(t, log, failFastRecentMessage, 0)

			// btpcli retries the command; its retry must not run the loop again.
			h0, c0 := base.counts()
			guardHandedUp(t, tr, guardRequest(t, hierSubaccount, "get"))
			if w := sl.got(); fmt.Sprint(w) != fmt.Sprint(guardedResendWaits[:1]) {
				t.Errorf("waits = %v, want only the first loop's", w)
			}
			if h, c := base.counts(); h != h0+1 || c != c0+2 {
				t.Errorf("hierarchy=%d commands=%d, want +1/+2", h-h0, c-c0)
			}
			kv := guardLine(t, log, failFastRecentMessage, 1)
			guardCheckKV(t, kv, map[string]string{"resends": "0"})
		})
	}
}

func TestGuardSleepError(t *testing.T) {
	tr, clk, base, sl, log := guardTransport(t)
	guardAnswer(t, tr, base, hierSubaccount, "create", "200")
	clk.advance(time.Second)
	var mu sync.Mutex
	var bodies []*failFastCountingBody
	base.setCommand(func() (*http.Response, error) {
		b := &failFastCountingBody{Reader: strings.NewReader("")}
		mu.Lock()
		bodies = append(bodies, b)
		mu.Unlock()
		return &http.Response{StatusCode: 500, Header: http.Header{}, Body: b}, nil
	})
	sl.err = context.Canceled
	_, c0 := base.counts()

	resp, err := tr.RoundTrip(guardRequest(t, hierSubaccount, "get"))
	if resp != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("RoundTrip = %v, %v, want nil, %v", resp, err, context.Canceled)
	}
	if _, c := base.counts(); c != c0+2 {
		t.Errorf("commands = %d, want %d (no send after the failed wait)", c-c0, 2)
	}
	mu.Lock()
	defer mu.Unlock()
	for i, b := range bodies {
		b.mu.Lock()
		if b.closes != 1 {
			t.Errorf("body %d closed %d times, want 1", i, b.closes)
		}
		b.mu.Unlock()
	}
	guardLine(t, log, failFastRecentMessage, 0)
}

func TestGuardHierarchyFailsDuringLoop(t *testing.T) {
	tr, clk, base, sl, log := guardTransport(t)
	guardAnswer(t, tr, base, hierSubaccount, "create", "200")
	clk.advance(time.Second)
	base.mu.Lock()
	base.onHierarchy = func() (*http.Response, error) { return hierResp(401, "", "unauthorized"), nil }
	base.mu.Unlock()
	base.setCommand(failFastBare500)
	h0, c0 := base.counts()

	guardHandedUp(t, tr, guardRequest(t, hierSubaccount, "get"))
	if w := sl.got(); fmt.Sprint(w) != fmt.Sprint(guardedResendWaits[:1]) {
		t.Errorf("waits = %v, want only the first", w)
	}
	// sendCommand's repeated call fails, then the loop's call fails.
	if h, c := base.counts(); h != h0+2 || c != c0+1 {
		t.Errorf("hierarchy=%d commands=%d, want 2/1", h-h0, c-c0)
	}
	kv := guardLine(t, log, failFastRecentMessage, 1)
	guardCheckKV(t, kv, map[string]string{"resends": "0"})
}

func TestGuardPerSubaccount(t *testing.T) {
	tr, clk, base, sl, _ := guardTransport(t)
	guardAnswer(t, tr, base, hierSubaccount, "create", "200")
	clk.advance(time.Second)
	base.setCommand(failFastBare500)

	resp, err := tr.RoundTrip(guardRequest(t, hierSubaccount2, "get"))
	failFastIsReplacement(t, resp, err)
	if w := sl.got(); len(w) != 0 {
		t.Errorf("waits = %v, want none", w)
	}
}

func TestGuardFailFastOff(t *testing.T) {
	tr, clk, base, sl, log := guardTransport(t)
	tr.failFast = false
	guardAnswer(t, tr, base, hierSubaccount, "create", "200")
	clk.advance(time.Second)
	base.setCommand(failFastBare500)

	guardHandedUp(t, tr, guardRequest(t, hierSubaccount, "get"))
	if w := sl.got(); len(w) != 0 {
		t.Errorf("waits = %v, want none", w)
	}
	guardLine(t, log, failFastRecentMessage, 0)
}

func TestGuardSleepContext(t *testing.T) {
	if err := sleepContext(context.Background(), 5*time.Millisecond); err != nil {
		t.Fatalf("sleepContext = %v, want nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepContext(ctx, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("sleepContext on a cancelled context = %v, want %v", err, context.Canceled)
	}
}

func TestGuardLogsNoSession(t *testing.T) {
	tr, clk, base, _, log := guardTransport(t)
	guardAnswer(t, tr, base, hierSubaccount, "create", "200")
	clk.advance(time.Second)
	base.setCommand(guardRefuse(3))
	if status, _ := hierRoundTrip(t, tr, guardRequest(t, hierSubaccount, "get")); status != 200 {
		t.Fatalf("recovered answer status = %d", status)
	}
	base.setCommand(failFastBare500)
	guardHandedUp(t, tr, guardRequest(t, hierSubaccount, "get"))

	guardLine(t, log, guardRecoveryMessage, 1)
	guardLine(t, log, failFastRecentMessage, 1)
	for _, e := range log.entries {
		if strings.Contains(e.msg, hierSession) || strings.Contains(fmt.Sprint(e.kv...), hierSession) {
			t.Fatalf("%s line %q leaks the session id: %v", e.level, e.msg, e.kv)
		}
	}
}

func TestGuardDefaults(t *testing.T) {
	l := newHierarchyLoader()
	if fmt.Sprint(l.resendWaits) != fmt.Sprint(guardedResendWaits) {
		t.Errorf("resendWaits = %v, want %v", l.resendWaits, guardedResendWaits)
	}
	if l.sleep == nil {
		t.Error("sleep is nil")
	}
}
