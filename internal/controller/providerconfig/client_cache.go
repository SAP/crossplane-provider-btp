package providerconfig

import (
	"crypto/sha256"
	"sync"
	"time"

	"github.com/sap/crossplane-provider-btp/btp"
)

const defaultEvictionTimeout = 2 * time.Hour

type cachedEntry struct {
	client     *btp.Client
	secretHash [32]byte
	lastUsed   time.Time
}

// ClientCache caches BTP clients per ProviderConfig name, invalidating only
// when the underlying secret content changes. Token refresh is handled
// automatically by the oauth2 library inside each cached HTTP client.
// Entries unused for longer than the eviction timeout are removed.
type ClientCache struct {
	mu              sync.RWMutex
	entries         map[string]*cachedEntry
	evictionTimeout time.Duration
}

// NewClientCache creates a new client cache.
func NewClientCache() *ClientCache {
	return &ClientCache{
		entries:         make(map[string]*cachedEntry),
		evictionTimeout: defaultEvictionTimeout,
	}
}

// GetOrCreate returns a cached client if the secret content hash matches.
// Otherwise it calls createFn to build a new client and caches it.
// Token refresh is handled transparently by the oauth2 transport inside the client.
// Stale entries unused for longer than the eviction timeout are removed on each call.
func (c *ClientCache) GetOrCreate(
	pcName string,
	cisSecret []byte,
	saSecret []byte,
	createFn func() (*btp.Client, error),
) (*btp.Client, error) {
	hash := computeHash(cisSecret, saSecret)
	now := time.Now()

	c.mu.RLock()
	entry, ok := c.entries[pcName]
	c.mu.RUnlock()

	if ok && entry.secretHash == hash {
		c.mu.Lock()
		entry.lastUsed = now
		c.evictExpiredLocked(now)
		c.mu.Unlock()
		return entry.client, nil
	}

	// Cache miss — create new client.
	// Note: On cold start, multiple goroutines may reach this point concurrently
	// for the same pcName, each creating their own client. This is acceptable —
	// last write wins, and the extra clients get GC'd. Holding the lock during
	// createFn() would block all controllers on a single client creation (which
	// involves network I/O on first use). In steady state, only one cached entry exists per pcName.
	client, err := createFn()
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[pcName] = &cachedEntry{
		client:     client,
		secretHash: hash,
		lastUsed:   now,
	}
	c.evictExpiredLocked(now)
	c.mu.Unlock()

	return client, nil
}

// evictExpiredLocked removes entries that haven't been used within the eviction timeout.
// Must be called while holding c.mu write lock.
func (c *ClientCache) evictExpiredLocked(now time.Time) {
	for name, entry := range c.entries {
		if now.Sub(entry.lastUsed) > c.evictionTimeout {
			delete(c.entries, name)
		}
	}
}

func computeHash(cisSecret []byte, saSecret []byte) [32]byte {
	h := sha256.New()
	h.Write(cisSecret)
	h.Write([]byte{0}) // separator
	h.Write(saSecret)
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// DefaultClientCache is the package-level cache used by CreateClient.
var DefaultClientCache = NewClientCache()
