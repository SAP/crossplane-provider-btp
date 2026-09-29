package tfclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
)

const (
	headerCLIBackendMediaType = "X-Cpcli-Backend-Mediatype"

	DefaultLookupCacheTTL   = 6 * time.Hour
	lookupCacheMaxEntries   = 4096
	lookupCacheMaxBodyBytes = 64 << 10
)

var lookupCacheTTL = DefaultLookupCacheTTL

// SetLookupCacheTTL sets how long a plan or offering lookup by id is served from
// the cache; 0 switches the cache off. It must be called before the first CLI
// client is built, i.e. from main().
func SetLookupCacheTTL(d time.Duration) { lookupCacheTTL = d }

// btpcli reads only these off an answer; the correlation id comes from the request.
var lookupReplayHeaders = []string{"Content-Type", headerCLIBackendStatus, headerCLIBackendMediaType}

type lookupCache struct {
	ttl        time.Duration
	maxEntries int
	maxBody    int
	now        func() time.Time // replaced in tests
	log        logging.Logger

	mu       sync.Mutex
	entries  map[string]*lookupEntry
	fullOnce sync.Once
}

// lookupEntry is never modified once stored, so a hit may read it without c.mu.
type lookupEntry struct {
	storedAt time.Time
	header   http.Header
	body     []byte
}

type lookupRequest struct {
	key        string
	subaccount string
}

func newLookupCache(ttl time.Duration, log logging.Logger) *lookupCache {
	if ttl <= 0 {
		return nil
	}
	return &lookupCache{
		ttl:        ttl,
		maxEntries: lookupCacheMaxEntries,
		maxBody:    lookupCacheMaxBodyBytes,
		now:        time.Now,
		log:        log,
		entries:    map[string]*lookupEntry{},
	}
}

// parse returns nil for anything that is not a plan or offering lookup by id.
// A lookup by name feeds the plan id of a create or update, so it must always
// be fresh.
func (c *lookupCache) parse(r *http.Request) *lookupRequest {
	if c == nil || r.Method != http.MethodPost || r.Header.Get(headerCLISessionId) == "" || r.GetBody == nil {
		return nil
	}
	_, rest, ok := strings.Cut(r.URL.Path, "/command/")
	if !ok {
		return nil
	}
	version, command, ok := strings.Cut(rest, "/")
	if !ok || version == "" || (command != "services/plan" && command != "services/offering") {
		return nil
	}
	if r.URL.RawQuery != "get" {
		return nil
	}

	body, err := r.GetBody()
	if err != nil {
		return nil
	}
	defer body.Close() //nolint:errcheck
	raw, err := io.ReadAll(io.LimitReader(body, maxCommandBodyBytes+1))
	if err != nil || len(raw) > maxCommandBodyBytes {
		return nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil || len(top) != 1 {
		return nil
	}
	var params map[string]json.RawMessage
	if err := json.Unmarshal(top["paramValues"], &params); err != nil || len(params) != 2 {
		return nil
	}
	var subaccount, id string
	if err := json.Unmarshal(params["subaccount"], &subaccount); err != nil || subaccount == "" {
		return nil
	}
	if err := json.Unmarshal(params["id"], &id); err != nil || id == "" {
		return nil
	}

	// The session id is in the key because an answer depends on what the logged-in
	// user may see; hashing keeps it out of the map in clear.
	h := sha256.New()
	for _, v := range []string{
		r.URL.Scheme, r.URL.Host, r.URL.Path, r.URL.RawQuery,
		r.Header.Get(headerCLIFormat), r.Header.Get(headerCLISubdomain),
		r.Header.Get(headerCLICustomIDP), r.Header.Get(headerCLISessionId),
		subaccount, id,
	} {
		h.Write([]byte(v))
		h.Write([]byte{0})
	}
	return &lookupRequest{key: hex.EncodeToString(h.Sum(nil)), subaccount: subaccount}
}

func (c *lookupCache) get(r *http.Request, q *lookupRequest) *http.Response {
	if r.Context().Err() != nil {
		return nil
	}
	c.mu.Lock()
	e, ok := c.entries[q.key]
	if !ok {
		c.mu.Unlock()
		return nil
	}
	age := c.now().Sub(e.storedAt)
	if age >= c.ttl {
		delete(c.entries, q.key)
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	if r.Body != nil {
		_ = r.Body.Close()
	}
	h := http.Header{}
	for k, v := range e.header {
		h[k] = append([]string(nil), v...)
	}
	corr := r.Header.Get(headerCorrelationID)
	if corr != "" {
		h.Set(headerCorrelationID, corr)
	}
	if c.log != nil {
		c.log.Debug("cli lookup served from cache",
			"cliServerURL", r.URL.Host,
			"url", r.URL.Host+r.URL.Path,
			"subaccount", q.subaccount,
			"ageMs", age.Milliseconds(),
			"correlationID", corr)
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        h,
		Body:          io.NopCloser(bytes.NewReader(e.body)),
		ContentLength: int64(len(e.body)),
		Request:       r,
	}
}

// put stores a successful answer. It reads the body ahead, so it replaces
// resp.Body with one that still hands the caller every byte and any read error.
func (c *lookupCache) put(q *lookupRequest, resp *http.Response) {
	if resp == nil || resp.StatusCode != http.StatusOK || resp.Header.Get(headerCLIBackendStatus) != "200" || resp.Body == nil {
		return
	}
	orig := resp.Body
	read, err := io.ReadAll(io.LimitReader(orig, int64(c.maxBody)+1))
	switch {
	case err != nil:
		resp.Body = &peekedBody{Reader: io.MultiReader(bytes.NewReader(read), errReader{err}), body: orig}
		return
	case len(read) > c.maxBody:
		resp.Body = &peekedBody{Reader: io.MultiReader(bytes.NewReader(read), orig), body: orig}
		return
	}
	resp.Body = &peekedBody{Reader: bytes.NewReader(read), body: orig}
	if !json.Valid(read) {
		return
	}

	h := http.Header{}
	for _, k := range lookupReplayHeaders {
		if v := resp.Header.Values(k); len(v) > 0 {
			h[k] = append([]string(nil), v...)
		}
	}
	e := &lookupEntry{storedAt: c.now(), header: h, body: read}

	full := false
	c.mu.Lock()
	if _, ok := c.entries[q.key]; !ok && len(c.entries) >= c.maxEntries {
		full = c.evictLocked()
	}
	c.entries[q.key] = e
	c.mu.Unlock()

	if full && c.log != nil {
		c.fullOnce.Do(func() {
			c.log.Info("cli lookup cache is full, dropping the oldest answers", "maxEntries", c.maxEntries)
		})
	}
}

// evictLocked makes room for one entry and reports whether a live one had to go.
// It must be called with c.mu held.
func (c *lookupCache) evictLocked() bool {
	now := c.now()
	for k, e := range c.entries {
		if now.Sub(e.storedAt) >= c.ttl {
			delete(c.entries, k)
		}
	}
	if len(c.entries) < c.maxEntries {
		return false
	}
	var oldestKey string
	var oldest time.Time
	for k, e := range c.entries {
		if oldestKey == "" || e.storedAt.Before(oldest) {
			oldestKey, oldest = k, e.storedAt
		}
	}
	delete(c.entries, oldestKey)
	return true
}

func (c *lookupCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// errReader hands a failed body read on to the caller after the bytes read before it.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }
