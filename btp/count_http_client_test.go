package btp

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type stubRT struct {
	calls int
	delay time.Duration
	mu    sync.Mutex
}

func (s *stubRT) RoundTrip(_ *http.Request) (*http.Response, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func TestCountingRoundTripper(t *testing.T) {
	ResetHTTPCallCounts()
	t.Cleanup(ResetHTTPCallCounts)

	stub := &stubRT{delay: 5 * time.Millisecond}
	rt := &CountingRoundTripper{Base: stub}

	for range 3 {
		req, _ := http.NewRequest("GET", "https://api.example.com/v1/foo/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee/bar", nil)
		_, _ = rt.RoundTrip(req)
	}
	req, _ := http.NewRequest("PUT", "https://api.example.com/v1/foo", nil)
	_, _ = rt.RoundTrip(req)

	stats := HTTPCallStats()
	get := stats["GET api.example.com/v1/foo/{id}/bar"]
	if get.Count != 3 {
		t.Errorf("GET count: got %d, want 3 (stats=%v)", get.Count, stats)
	}
	if get.Max < 5*time.Millisecond {
		t.Errorf("GET max: got %s, want >=5ms", get.Max)
	}
	if get.Total < 15*time.Millisecond {
		t.Errorf("GET total: got %s, want >=15ms", get.Total)
	}
	put := stats["PUT api.example.com/v1/foo"]
	if put.Count != 1 {
		t.Errorf("PUT count: got %d, want 1 (stats=%v)", put.Count, stats)
	}
	if stub.calls != 4 {
		t.Errorf("base RT calls: got %d, want 4", stub.calls)
	}
}
