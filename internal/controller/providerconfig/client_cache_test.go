package providerconfig

import (
	"sync"
	"testing"
	"time"

	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/stretchr/testify/assert"
)

func TestClientCache_GetOrCreate_CacheHit(t *testing.T) {
	cache := NewClientCache()

	callCount := 0
	createFn := func() (*btp.Client, error) {
		callCount++
		return &btp.Client{}, nil
	}

	cis := []byte("cis-secret-data")
	sa := []byte("sa-secret-data")

	client1, err := cache.GetOrCreate("pc1", cis, sa, createFn)
	assert.NoError(t, err)
	assert.NotNil(t, client1)
	assert.Equal(t, 1, callCount)

	// Second call with same data should return cached client
	client2, err := cache.GetOrCreate("pc1", cis, sa, createFn)
	assert.NoError(t, err)
	assert.Same(t, client1, client2)
	assert.Equal(t, 1, callCount) // createFn not called again
}

func TestClientCache_GetOrCreate_DifferentProviderConfigs(t *testing.T) {
	cache := NewClientCache()

	callCount := 0
	createFn := func() (*btp.Client, error) {
		callCount++
		return &btp.Client{}, nil
	}

	cis := []byte("cis-secret-data")
	sa := []byte("sa-secret-data")

	client1, err := cache.GetOrCreate("pc1", cis, sa, createFn)
	assert.NoError(t, err)
	assert.NotNil(t, client1)

	// Different provider config name → cache miss
	client2, err := cache.GetOrCreate("pc2", cis, sa, createFn)
	assert.NoError(t, err)
	assert.NotNil(t, client2)
	assert.Equal(t, 2, callCount)
}

func TestClientCache_GetOrCreate_SecretChange(t *testing.T) {
	cache := NewClientCache()

	callCount := 0
	createFn := func() (*btp.Client, error) {
		callCount++
		return &btp.Client{}, nil
	}

	cis := []byte("cis-secret-data")
	sa := []byte("sa-secret-data")

	client1, err := cache.GetOrCreate("pc1", cis, sa, createFn)
	assert.NoError(t, err)
	assert.NotNil(t, client1)
	assert.Equal(t, 1, callCount)

	// Changed CIS secret → cache miss, new client created
	newCis := []byte("cis-secret-data-rotated")
	client2, err := cache.GetOrCreate("pc1", newCis, sa, createFn)
	assert.NoError(t, err)
	assert.NotNil(t, client2)
	assert.NotSame(t, client1, client2)
	assert.Equal(t, 2, callCount)
}

func TestClientCache_GetOrCreate_SASecretChange(t *testing.T) {
	cache := NewClientCache()

	callCount := 0
	createFn := func() (*btp.Client, error) {
		callCount++
		return &btp.Client{}, nil
	}

	cis := []byte("cis-secret-data")
	sa := []byte("sa-secret-data")

	_, err := cache.GetOrCreate("pc1", cis, sa, createFn)
	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Changed SA secret → cache miss
	newSa := []byte("sa-secret-data-rotated")
	_, err = cache.GetOrCreate("pc1", cis, newSa, createFn)
	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestClientCache_GetOrCreate_ConcurrentAccess(t *testing.T) {
	cache := NewClientCache()

	var callCount int
	var mu sync.Mutex
	createFn := func() (*btp.Client, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		return &btp.Client{}, nil
	}

	cis := []byte("cis-secret-data")
	sa := []byte("sa-secret-data")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cache.GetOrCreate("pc1", cis, sa, createFn)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	// Due to race between RUnlock and Lock, a few goroutines may create clients,
	// but the total should be much less than 100
	mu.Lock()
	assert.Less(t, callCount, 100)
	mu.Unlock()
}

func TestClientCache_GetOrCreate_EvictsStaleEntries(t *testing.T) {
	cache := NewClientCache()
	cache.evictionTimeout = 1 * time.Millisecond

	createFn := func() (*btp.Client, error) {
		return &btp.Client{}, nil
	}

	cis := []byte("cis-secret-data")
	sa := []byte("sa-secret-data")

	// Create two entries
	_, err := cache.GetOrCreate("pc1", cis, sa, createFn)
	assert.NoError(t, err)
	_, err = cache.GetOrCreate("pc2", cis, sa, createFn)
	assert.NoError(t, err)

	assert.Len(t, cache.entries, 2)

	// Wait for eviction timeout
	time.Sleep(2 * time.Millisecond)

	// Access pc1 only — this triggers eviction sweep and refreshes pc1's lastUsed
	_, err = cache.GetOrCreate("pc1", cis, sa, createFn)
	assert.NoError(t, err)

	// pc2 should have been evicted, pc1 kept (it was just accessed)
	cache.mu.RLock()
	_, pc1Exists := cache.entries["pc1"]
	_, pc2Exists := cache.entries["pc2"]
	cache.mu.RUnlock()

	assert.True(t, pc1Exists)
	assert.False(t, pc2Exists)
}
