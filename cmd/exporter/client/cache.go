package client

import (
	"sync"
	"time"
)

// ObjectCache for BTP resources to avoid repeated CLI calls.
type ObjectCache struct {
	mu   sync.RWMutex
	ttl  time.Duration
	done bool

	// objects are objects in BTP CLI sense, e.g. returned by BTP CLI.
	objects map[string]*object
}

// object is a single BTP resource, e.g. a subaccount or a service instance.
type object struct {
	value      interface{}
	expiration time.Time
}

func New(ttl time.Duration) *ObjectCache {
	c := &ObjectCache{
		objects: make(map[string]*object),
		ttl:     ttl,
	}
	go c.cleanupExpired()
	return c
}

func (c *ObjectCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.objects[key] = &object{
		value:      value,
		expiration: time.Now().Add(c.ttl),
	}
}

func (c *ObjectCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	obj, exists := c.objects[key]
	if !exists || time.Now().After(obj.expiration) {
		return nil, false
	}

	return obj.value, true
}

func (c *ObjectCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.objects, key)
}

func (c *ObjectCache) Close() {
	c.mu.Lock()
	c.done = true
	c.mu.Unlock()
}

func (c *ObjectCache) cleanupExpired() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			if c.done {
				c.mu.Unlock()
				return
			}
			now := time.Now()
			for k, v := range c.objects {
				if now.After(v.expiration) {
					delete(c.objects, k)
				}
			}
			c.mu.Unlock()
		}
	}
}
