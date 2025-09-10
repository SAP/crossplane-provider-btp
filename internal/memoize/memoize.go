// Memoization helper package. It defines the Map type that can be
// used for memoization purposes.
//
// When a function is pure, you can use the Map type to memoize it.
//
// Let's say, the function has the following declaration:
//
//  func(input InputType) OutputType
//
// This func can be memoized like this:
//
//  func(input InputType) OutputType {
//    result, found := map.Get(input)
//    if !found {
//      result := calculateResult(input)
//      map.Set(input, result)
//    }
//    return result
//  }
package memoize

import (
	"context"
	"sync"
	"time"
)

type mapValue[V any] struct {
	v         *V
	createdAt time.Time
}

// Map type implements a caching map. The caching map is a map data
// type where the keys are of type Key and the values are of generic
// types.
//
// The keys in the map have an expiration time. There is a garbage
// collection process, that periodically cleans the expired keys.
//
// There is an upper limit, how many keys the map can store.
type Map[V any] struct {
	m             map[string]mapValue[V]
	stopped       bool
	cancel        context.CancelFunc
	lock          sync.RWMutex
	keyExpiration time.Duration
	keyLimit      int
	gcFrequency   time.Duration
}

// Key is an interface that defines the methods that the Map keys must
// implement.
type Key interface {
	// GetMemoKey returns *string that can be used as the Map
	// key. The key is ignored, if the GetMemoKey method returns
	// nil.
	GetMemoKey() *string
}

// New function creates a new Map value. The provided ctx can be used
// to stop the goroutines of the returned Map value.
func New[V any](ctx context.Context) *Map[V] {
	ctx, cancel := context.WithCancel(ctx)
	m := &Map[V]{
		m:             map[string]mapValue[V]{},
		stopped:       false,
		cancel:        cancel,
		lock:          sync.RWMutex{},
		keyExpiration: DefaultKeyExpiration,
		keyLimit:      DefaultKeyLimit,
		gcFrequency:   DefaultGCFrequency,
	}
	m.gc(ctx)
	return m
}

// WithKeyExpiration method sets the key expiration value of a Map value.
func (m *Map[V]) WithKeyExpiration(expiration time.Duration) *Map[V] {
	m.keyExpiration = expiration
	return m
}

// WithGCFrequence method sets the garbage collection frequence of a
// Map value.
func (m *Map[V]) WithGCFrequency(frequency time.Duration) *Map[V] {
	m.gcFrequency = frequency
	return m
}

// WithKeyLimit method sets the key limit of a map value. If the key
// limit gets lower than the number of keys in the map, new elements
// will not be added to the map until some old keys get purged out.
func (m *Map[V]) WithKeyLimit(limit int) *Map[V] {
	m.keyLimit = limit
	return m
}

func (m *Map[V]) gc(ctx context.Context) {
	go func() {
	GcLoop:
		for {
			freq := max(m.gcFrequency, MinimumGCFrequency)
			timer := time.NewTimer(freq)
			select {
			case _, ok := <-timer.C:
				if !ok {
					// timer is stopped, this shall not happen
					continue GcLoop
				}
				m.lock.Lock()
				now := time.Now()
				for k, v := range m.m {
					if now.Sub(v.createdAt) > m.keyExpiration {
						// key is expired
						delete(m.m, k)
					}
				}
				m.lock.Unlock()
			case <-ctx.Done():
				// We'll stop the GC loop and free all pending resources
				timer.Stop()
				return
			}
		}
	}()
}

// Stop method frees the allocated goroutins of a Map value. After a
// Map is stopped, Get, Set, etc. operations stop working.
func (m *Map[V]) Stop() {
	m.cancel()
	m.stopped = true
}

// Get method returns a value, which has been previously stored with
// the provided key, unless the key is expired.
func (m *Map[V]) Get(key Key) (*V, bool) {
	if m.stopped {
		return nil, false
	}
	k := key.GetMemoKey()
	if k == nil {
		// The map has no such key
		return nil, false
	}
	m.lock.RLock()
	cdAt, found := m.m[*k]
	m.lock.RUnlock()
	if !found {
		// There is no ConnectionDetails in the hashmap
		return nil, false
	}
	if time.Since(cdAt.createdAt) > m.keyExpiration {
		// ConnectionDetails is expired, we have to ask for a
		// new value
		m.lock.Lock()
		delete(m.m, *k)
		m.lock.Unlock()
		return nil, false
	}
	// Get the value from the hash
	return cdAt.v, true
}

// Set method stores a value for the provided key. If the value is
// nil, the key is deleted. If the map is full (the number of stored
// keys has reached the key limit), the Set operation is a no-op.
func (m *Map[V]) Set(key Key, value *V) {
	if m.stopped {
		return
	}
	k := key.GetMemoKey()
	if k == nil {
		// We can't store a ConnectionDetails if the instance has no Labels
		return
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	if value == nil {
		// value == nil indicates that we want to delete the key
		delete(m.m, *k)
		return
	}
	if len(m.m) >= m.keyLimit {
		return
	}
	m.m[*k] = mapValue[V]{
		v:         value,
		createdAt: time.Now(),
	}
}

// Invalidate method removes the provided key from the Map object.
func (m *Map[V]) Invalidate(key Key) {
	if m.stopped {
		return
	}
	k := key.GetMemoKey()
	if k == nil {
		// We can't invalidate a key if it has no string representation
		return
	}
	m.lock.Lock()
	delete(m.m, *k)
	m.lock.Unlock()
}

// DefaultKeyExpiration sets the default key expiration value for
// any new Map objects.
var DefaultKeyExpiration = 5 * time.Minute

// DefaultKeyLimit sets the default key limit value for any new Map
// objects.
var DefaultKeyLimit = 5000

// DefaultGCFrequency sets the default garbage collection frequencye
// for any new Map objects.
var DefaultGCFrequency = 5 * time.Minute

// MinimumGCFrequency show the minimum value of the garbage collection
// frequency.
const MinimumGCFrequency = 1 * time.Minute
