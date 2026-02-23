package resources

import "sync"

// Registry holds IDs of all exported resources.
type Registry struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

// NewRegistry creates a new instance of Registry.
func NewRegistry() *Registry {
	return &Registry{
		seen: make(map[string]struct{}),
	}
}

// Register registers a new resource ID in the export Registry.
// It returns true if the ID was not registered before, and false otherwise.
// This is used in particular to avoid exporting the same reusable resource (e.g. Service Manager) multiple times,
// when it is referenced by multiple other resources.
func (r *Registry) Register(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.seen[id]; exists {
		return false
	}
	r.seen[id] = struct{}{}

	return true
}
