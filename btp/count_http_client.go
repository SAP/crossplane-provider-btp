package btp

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
)

// CountingRoundTripper records count + latency (sum, max) per
// (method,host,bucketed-path) for every request it forwards. One atomic add
// each for count/sum/max; safe to leave on. Inspect with HTTPCallStats() or
// LogAndResetHTTPCallCounts(); zero with ResetHTTPCallCounts().
//
// ponytail: package-level stats map. Switch to per-client stats if you need
// to isolate multiple BTP credentials in the same process.
type CountingRoundTripper struct {
	Base http.RoundTripper
}

type callStat struct {
	count     atomic.Int64
	sumNs     atomic.Int64
	maxNs     atomic.Int64
	reqBytes  atomic.Int64
	respBytes atomic.Int64
}

// CallStat is the read-only view returned by HTTPCallStats.
type CallStat struct {
	Count     int64
	Total     time.Duration
	Max       time.Duration
	ReqBytes  int64
	RespBytes int64
}

var (
	httpStats sync.Map // string → *callStat
	uuidRe    = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
)

func (c *CountingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	key := req.Method + " " + req.URL.Host + uuidRe.ReplaceAllString(req.URL.Path, "{id}")
	v, _ := httpStats.LoadOrStore(key, &callStat{})
	st := v.(*callStat)

	base := c.Base
	if base == nil {
		base = http.DefaultTransport
	}
	start := time.Now()
	resp, err := base.RoundTrip(req)
	d := time.Since(start).Nanoseconds()

	st.count.Add(1)
	st.sumNs.Add(d)
	for {
		cur := st.maxNs.Load()
		if d <= cur || st.maxNs.CompareAndSwap(cur, d) {
			break
		}
	}
	// ponytail: bandwidth via Content-Length when set; for chunked HTTP/2
	// responses (BTP uses this) Content-Length is -1, so we wrap resp.Body
	// with a counter that records bytes as the caller drains it.
	if req.ContentLength > 0 {
		st.reqBytes.Add(req.ContentLength)
	}
	if resp != nil {
		if resp.ContentLength > 0 {
			st.respBytes.Add(resp.ContentLength)
		} else if resp.Body != nil {
			resp.Body = &countingReadCloser{rc: resp.Body, total: &st.respBytes}
		}
	}
	return resp, err
}

type countingReadCloser struct {
	rc    io.ReadCloser
	total *atomic.Int64
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	n, err := c.rc.Read(p)
	if n > 0 {
		c.total.Add(int64(n))
	}
	return n, err
}

func (c *countingReadCloser) Close() error { return c.rc.Close() }

// HTTPCallStats returns a snapshot of per-key call stats since the last reset.
func HTTPCallStats() map[string]CallStat {
	out := map[string]CallStat{}
	httpStats.Range(func(k, v any) bool {
		st := v.(*callStat)
		out[k.(string)] = CallStat{
			Count:     st.count.Load(),
			Total:     time.Duration(st.sumNs.Load()),
			Max:       time.Duration(st.maxNs.Load()),
			ReqBytes:  st.reqBytes.Load(),
			RespBytes: st.respBytes.Load(),
		}
		return true
	})
	return out
}

// HTTPCallCounts returns just the call counts (back-compat).
func HTTPCallCounts() map[string]int64 {
	out := map[string]int64{}
	for k, v := range HTTPCallStats() {
		out[k] = v.Count
	}
	return out
}

// ResetHTTPCallCounts zeros all stats.
func ResetHTTPCallCounts() {
	httpStats.Range(func(k, _ any) bool {
		httpStats.Delete(k)
		return true
	})
}

// LogAndResetHTTPCallCounts dumps a sorted snapshot via the given logger and
// resets the stats. Each key's value is "count avg=<ms> max=<ms> in=<bytes> out=<bytes>".
func LogAndResetHTTPCallCounts(logger logging.Logger, label string) {
	snap := HTTPCallStats()
	if len(snap) == 0 {
		return
	}
	keys := make([]string, 0, len(snap))
	for k := range snap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	args := []any{"label", label}
	for _, k := range keys {
		s := snap[k]
		avg := time.Duration(0)
		if s.Count > 0 {
			avg = time.Duration(int64(s.Total) / s.Count)
		}
		args = append(args, k, fmt.Sprintf(
			"n=%d avg=%s max=%s req=%s resp=%s",
			s.Count, roundMs(avg), roundMs(s.Max),
			humanBytes(s.ReqBytes), humanBytes(s.RespBytes),
		))
	}
	logger.Info("BTP HTTP call counts", args...)
	ResetHTTPCallCounts()
}

func roundMs(d time.Duration) time.Duration { return d.Round(time.Millisecond) }

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2fGB", float64(n)/float64(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.2fMB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/float64(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
