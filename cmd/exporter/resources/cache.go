package resources

import (
	"fmt"
	"iter"
	"maps"
	"regexp"
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

// KeepSelectedOnly keeps only resources that match the values selected by the user.
// The selected values can be either resource IDs, display names, or regular expressions matching display names.
func (c *resourceCache[T]) KeepSelectedOnly(selectedValues []string) {
	displayValues := c.ValuesForSelection()

	// Prepare set of resource IDs to keep.
	// Also, if a selected value is neither a resource ID nor a display value, compile regexes for name matching.
	var nameRxs []*regexp.Regexp
	keepSet := make(map[string]bool)
	for _, v := range selectedValues {
		if resourceId, ok := displayValues.values[v]; ok {
			// Selected by display value.
			keepSet[resourceId] = true
		} else if _, ok := c.resources[v]; ok {
			// Selected by resource ID.
			keepSet[v] = true
		} else if rx, err := regexp.Compile(v); err == nil {
			nameRxs = append(nameRxs, rx)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Match resources by name regexes, if any.
	for _, rx := range nameRxs {
		for key := range c.resources {
			if rx.MatchString(c.resources[key].GetDisplayName()) {
				keepSet[key] = true
			}
		}
	}

	// Remove resources not in the keep set.
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
