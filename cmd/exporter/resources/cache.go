package resources

import (
	"fmt"
	"iter"
	"maps"
	"slices"
	"sync"
)

type ResourceCache[T BtpResource] interface {
	Set(resource T)
	Get(id string) T
	Len() int
	Store(resources ...T)
	All() iter.Seq2[string, T]
	AllIDs() []string
	ValuesForSelection() *DisplayValues
	KeepSelectedOnly(selectedValues []string)
}

// ResourceCache for BTP resources to avoid repeated CLI calls.
type resourceCache[T BtpResource] struct {
	mu sync.RWMutex

	// BTP resources returned by BTP CLI.
	resources map[string]T
}

func NewResourceCache[T BtpResource]() ResourceCache[T] {
	c := &resourceCache[T]{
		resources: make(map[string]T),
	}
	return c
}

func (c *resourceCache[T]) Set(value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.resources[value.GetID()] = value
}

func (c *resourceCache[T]) Get(key string) T {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.resources[key]
}

func (c *resourceCache[T]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.resources)
}

func (c *resourceCache[T]) Store(resources ...T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, resource := range resources {
		c.resources[resource.GetID()] = resource
	}
}

func (c *resourceCache[T]) All() iter.Seq2[string, T] {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return maps.All(c.resources)
}

func (c *resourceCache[T]) AllIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return slices.Sorted(maps.Keys(c.resources))
}

func (c *resourceCache[T]) ValuesForSelection() *DisplayValues {
	c.mu.RLock()
	defer c.mu.RUnlock()

	values := NewDisplayValues()
	for _, r := range c.resources {
		values.Set(fmt.Sprintf("%s - [%s]", r.GetDisplayName(), r.GetID()), r.GetID())
	}
	return values
}

func (c *resourceCache[T]) KeepSelectedOnly(selectedValues []string) {
	displayValues := c.ValuesForSelection()

	keepSet := make(map[string]bool)
	for _, v := range selectedValues {
		if resourceId, ok := displayValues.values[v]; ok {
			keepSet[resourceId] = true
		} else if _, ok := c.resources[v]; ok {
			keepSet[v] = true
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.resources {
		if !keepSet[key] {
			delete(c.resources, key)
		}
	}
}

type DisplayValues struct {
	// The key is the display values shown to the user.
	// The value is the ID of the resource to track back after selection.
	values map[string]string
}

func NewDisplayValues() *DisplayValues {
	return &DisplayValues{
		values: make(map[string]string),
	}
}

func (d *DisplayValues) Set(displayName, resourceID string) {
	d.values[displayName] = resourceID
}

func (d *DisplayValues) Values() []string {
	return slices.Sorted(maps.Keys(d.values))
}
