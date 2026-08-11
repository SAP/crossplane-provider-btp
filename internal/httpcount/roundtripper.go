// Package httpcount provides an http.RoundTripper that counts requests
// flowing through it. Intended as a test helper for asserting caching /
// short-circuit behavior of higher-level clients.
package httpcount

import (
	"maps"
	"net/http"
	"sync"
	"sync/atomic"
)

// RoundTripper wraps a base http.RoundTripper and counts every RoundTrip
// call. Safe for concurrent use. A nil Base falls back to
// http.DefaultTransport.
type RoundTripper struct {
	Base http.RoundTripper

	total atomic.Uint64

	mu     sync.RWMutex
	byKey  map[string]uint64 // METHOD + " " + URL.Path
	byHost map[string]uint64
}

// New returns a fresh RoundTripper. Pass base = nil to wrap the default
// transport.
func New(base http.RoundTripper) *RoundTripper {
	return &RoundTripper{
		Base:   base,
		byKey:  make(map[string]uint64),
		byHost: make(map[string]uint64),
	}
}

// Client returns an *http.Client with the counting RoundTripper installed
// as its Transport. Convenience for tests.
func (r *RoundTripper) Client() *http.Client {
	return &http.Client{Transport: r}
}

// RoundTrip implements http.RoundTripper. It records the request before
// delegating, so counts include failed / cancelled requests too — the
// question "did we go to the network?" is answered by intent, not by
// success.
func (r *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.total.Add(1)

	key := req.Method + " " + req.URL.Path
	host := req.URL.Host

	r.mu.Lock()
	r.byKey[key]++
	r.byHost[host]++
	r.mu.Unlock()

	base := r.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// Total returns the total number of RoundTrip calls observed.
func (r *RoundTripper) Total() uint64 {
	return r.total.Load()
}

// CountFor returns the count for a specific "METHOD /path" key.
func (r *RoundTripper) CountFor(method, path string) uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byKey[method+" "+path]
}

// CountForHost returns the count for a specific host.
func (r *RoundTripper) CountForHost(host string) uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byHost[host]
}

// Snapshot returns a copy of the per-key counts.
func (r *RoundTripper) Snapshot() map[string]uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]uint64, len(r.byKey))
	maps.Copy(out, r.byKey)
	return out
}

// Reset zeroes all counters. Useful between phases of a single test.
func (r *RoundTripper) Reset() {
	r.total.Store(0)
	r.mu.Lock()
	clear(r.byKey)
	clear(r.byHost)
	r.mu.Unlock()
}
