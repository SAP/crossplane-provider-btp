package resources

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// Mock BtpResource for testing
type mockResource struct {
	id          string
	displayName string
}

func (m *mockResource) GetID() string {
	return m.id
}

func (m *mockResource) GetDisplayName() string {
	return m.displayName
}

func (m *mockResource) GetExternalName() string {
	return ""
}

func (m *mockResource) GenerateK8sResourceName() string {
	return ""
}

func TestResourceCache_KeepSelectedOnly(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	tests := []struct {
		name           string
		initialCache   []*mockResource
		selectedValues []string
		wantIDs        []string
	}{
		{
			name: "select by resource ID",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
				{id: "id-2", displayName: "Resource Two"},
				{id: "id-3", displayName: "Resource Three"},
			},
			selectedValues: []string{"id-1", "id-3"},
			wantIDs:        []string{"id-1", "id-3"},
		},
		{
			name: "select by display value",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
				{id: "id-2", displayName: "Resource Two"},
				{id: "id-3", displayName: "Resource Three"},
			},
			selectedValues: []string{"Resource One - [id-1]", "Resource Three - [id-3]"},
			wantIDs:        []string{"id-1", "id-3"},
		},
		{
			name: "select by regex matching display name",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "production-service"},
				{id: "id-2", displayName: "development-service"},
				{id: "id-3", displayName: "staging-app"},
				{id: "id-4", displayName: "test-service"},
			},
			selectedValues: []string{".*service$"},
			wantIDs:        []string{"id-1", "id-2", "id-4"},
		},
		{
			name: "mixed selection methods",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "prod-api"},
				{id: "id-2", displayName: "dev-api"},
				{id: "id-3", displayName: "staging-web"},
				{id: "id-4", displayName: "test-web"},
			},
			selectedValues: []string{
				"id-1",             // by ID
				"dev-api - [id-2]", // by display value
				".*-web$",          // by regex
			},
			wantIDs: []string{"id-1", "id-2", "id-3", "id-4"},
		},
		{
			name: "non-existent key does not override existing one",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
				{id: "id-2", displayName: "Resource Two"},
			},
			selectedValues: []string{"non-existent-id", "id-2"},
			wantIDs:        []string{"id-2"},
		},
		{
			name: "non-existent selection keeps nothing",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
				{id: "id-2", displayName: "Resource Two"},
			},
			selectedValues: []string{"non-existent-id", "non-existent-name"},
			wantIDs:        []string{},
		},
		{
			name: "empty selection keeps nothing",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
				{id: "id-2", displayName: "Resource Two"},
			},
			selectedValues: []string{},
			wantIDs:        []string{},
		},
		{
			name: "regex with special characters",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "service.btp.sap"},
				{id: "id-2", displayName: "service-btp-sap"},
				{id: "id-3", displayName: "service_btp_sap"},
			},
			selectedValues: []string{`service\.btp\.sap`},
			wantIDs:        []string{"id-1"},
		},
		{
			name: "overlapping selections",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "api-service"},
				{id: "id-2", displayName: "web-service"},
			},
			selectedValues: []string{
				"id-1",
				"api-service - [id-1]",
				".*-service$",
			},
			wantIDs: []string{"id-1", "id-2"},
		},
		{
			name: "invalid regex is ignored",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
				{id: "id-2", displayName: "Resource Two"},
			},
			selectedValues: []string{"[invalid(regex", "id-1"},
			wantIDs:        []string{"id-1"},
		},
		{
			name: "case sensitive matching",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Production"},
				{id: "id-2", displayName: "production"},
			},
			selectedValues: []string{"^Production$"},
			wantIDs:        []string{"id-1"},
		},
		{
			name:           "empty cache",
			initialCache:   []*mockResource{},
			selectedValues: []string{"any-value"},
			wantIDs:        []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewResourceCache[*mockResource]()
			cache.Store(tt.initialCache...)

			cache.KeepSelectedOnly(tt.selectedValues)

			gotIDs := cache.AllIDs()
			r.Equal(len(tt.wantIDs), len(gotIDs), "number of resources mismatch")
			r.ElementsMatch(tt.wantIDs, gotIDs, "resource IDs mismatch")
		})
	}
}

func TestResourceCache_KeepSelectedOnly_Concurrency(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	cache := NewResourceCache[*mockResource]()
	cache.Store([]*mockResource{
		{id: "id-1", displayName: "Resource One"},
		{id: "id-2", displayName: "Resource Two"},
		{id: "id-3", displayName: "Resource Three"},
	}...)

	var wg sync.WaitGroup

	// Concurrent operations with DIFFERENT filters
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.KeepSelectedOnly([]string{"id-1"}) // Keep only id-1
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.KeepSelectedOnly([]string{"id-2"}) // Keep only id-2
		}()
	}

	wg.Wait()

	// The go routines above effectively clear the cache
	finalLen := cache.Len()
	r.Equal(0, finalLen, "cache should contain exactly 1 resource after concurrent operations")
	r.Nil(cache.AllIDs())
	elementCount := 0
	for range cache.All() {
		elementCount++
	}
	r.Equal(0, elementCount, "cache.All() should yield no elements")
}

func TestResourceCache_ValuesForSelection(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	tests := []struct {
		name         string
		resources    []*mockResource
		wantValues   []string
		wantMappings map[string]string
	}{
		{
			name: "single resource",
			resources: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
			},
			wantValues: []string{"Resource One - [id-1]"},
			wantMappings: map[string]string{
				"Resource One - [id-1]": "id-1",
			},
		},
		{
			name: "multiple resources",
			resources: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
				{id: "id-2", displayName: "Resource Two"},
				{id: "id-3", displayName: "Resource Three"},
			},
			wantValues: []string{
				"Resource One - [id-1]",
				"Resource Three - [id-3]",
				"Resource Two - [id-2]",
			},
			wantMappings: map[string]string{
				"Resource One - [id-1]":   "id-1",
				"Resource Two - [id-2]":   "id-2",
				"Resource Three - [id-3]": "id-3",
			},
		},
		{
			name:         "empty cache",
			resources:    []*mockResource{},
			wantValues:   nil,
			wantMappings: map[string]string{},
		},
		{
			name: "resources with special characters in display name",
			resources: []*mockResource{
				{id: "id-1", displayName: "service.btp.sap"},
				{id: "id-2", displayName: "service-btp-sap"},
				{id: "id-3", displayName: "service_btp_sap"},
			},
			wantValues: []string{
				"service-btp-sap - [id-2]",
				"service.btp.sap - [id-1]",
				"service_btp_sap - [id-3]",
			},
			wantMappings: map[string]string{
				"service.btp.sap - [id-1]": "id-1",
				"service-btp-sap - [id-2]": "id-2",
				"service_btp_sap - [id-3]": "id-3",
			},
		},
		{
			name: "resources with duplicate display names",
			resources: []*mockResource{
				{id: "id-1", displayName: "Duplicate Name"},
				{id: "id-2", displayName: "Duplicate Name"},
			},
			wantValues: []string{
				"Duplicate Name - [id-1]",
				"Duplicate Name - [id-2]",
			},
			wantMappings: map[string]string{
				"Duplicate Name - [id-1]": "id-1",
				"Duplicate Name - [id-2]": "id-2",
			},
		},
		{
			name: "resources with empty display name",
			resources: []*mockResource{
				{id: "id-1", displayName: ""},
				{id: "id-2", displayName: "Resource Two"},
			},
			wantValues: []string{
				" - [id-1]",
				"Resource Two - [id-2]",
			},
			wantMappings: map[string]string{
				" - [id-1]":             "id-1",
				"Resource Two - [id-2]": "id-2",
			},
		},
		{
			name: "resources with brackets in display name",
			resources: []*mockResource{
				{id: "id-1", displayName: "Resource [with brackets]"},
				{id: "id-2", displayName: "Resource (with parentheses)"},
			},
			wantValues: []string{
				"Resource (with parentheses) - [id-2]",
				"Resource [with brackets] - [id-1]",
			},
			wantMappings: map[string]string{
				"Resource [with brackets] - [id-1]":    "id-1",
				"Resource (with parentheses) - [id-2]": "id-2",
			},
		},
		{
			name: "resources with unicode characters",
			resources: []*mockResource{
				{id: "id-1", displayName: "Resource 日本語"},
				{id: "id-2", displayName: "Resource Ñoño"},
			},
			wantValues: []string{
				"Resource Ñoño - [id-2]",
				"Resource 日本語 - [id-1]",
			},
			wantMappings: map[string]string{
				"Resource 日本語 - [id-1]": "id-1",
				"Resource Ñoño - [id-2]":   "id-2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewResourceCache[*mockResource]()
			cache.Store(tt.resources...)

			displayValues := cache.ValuesForSelection()
			r.NotNil(displayValues)

			// Verify values are sorted
			gotValues := displayValues.Values()
			r.ElementsMatch(tt.wantValues, gotValues, "display values mismatch")
			r.Equal(tt.wantValues, gotValues, "display values should be sorted")

			// Verify mappings
			for displayValue, expectedID := range tt.wantMappings {
				actualID := displayValues.values[displayValue]
				r.Equal(expectedID, actualID, "mapping mismatch for display value: %s", displayValue)
			}

			// Verify no extra mappings
			r.Equal(len(tt.wantMappings), len(displayValues.values), "unexpected number of mappings")
		})
	}
}

func TestResourceCache_Copy(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	tests := []struct {
		name         string
		initialCache []*mockResource
		wantIDs      []string
	}{
		{
			name: "copy with multiple resources",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
				{id: "id-2", displayName: "Resource Two"},
				{id: "id-3", displayName: "Resource Three"},
			},
			wantIDs: []string{"id-1", "id-2", "id-3"},
		},
		{
			name: "copy with single resource",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
			},
			wantIDs: []string{"id-1"},
		},
		{
			name:         "copy empty cache",
			initialCache: []*mockResource{},
			wantIDs:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewResourceCache[*mockResource]()
			cache.Store(tt.initialCache...)

			copiedCache := cache.Copy()

			// Verify the copied cache has the same resources
			r.Equal(cache.Len(), copiedCache.Len(), "copied cache should have same length")
			r.ElementsMatch(tt.wantIDs, copiedCache.AllIDs(), "copied cache should have same IDs")

			// Verify each resource is accessible from the copy
			for _, resource := range tt.initialCache {
				copiedResource := copiedCache.Get(resource.id)
				r.NotNil(copiedResource, "resource should exist in copied cache")
				r.Equal(resource.id, copiedResource.GetID())
				r.Equal(resource.displayName, copiedResource.GetDisplayName())
			}
		})
	}
}

func TestResourceCache_Copy_Independence(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	tests := []struct {
		name            string
		initialCache    []*mockResource
		modifyOriginal  func(cache ResourceCache[*mockResource])
		modifyCopy      func(cache ResourceCache[*mockResource])
		wantOriginalIDs []string
		wantCopiedIDs   []string
	}{
		{
			name: "adding to original does not affect copy",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
			},
			modifyOriginal: func(cache ResourceCache[*mockResource]) {
				cache.Set(&mockResource{id: "id-2", displayName: "Resource Two"})
			},
			modifyCopy:      func(cache ResourceCache[*mockResource]) {},
			wantOriginalIDs: []string{"id-1", "id-2"},
			wantCopiedIDs:   []string{"id-1"},
		},
		{
			name: "adding to copy does not affect original",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
			},
			modifyOriginal: func(cache ResourceCache[*mockResource]) {},
			modifyCopy: func(cache ResourceCache[*mockResource]) {
				cache.Set(&mockResource{id: "id-2", displayName: "Resource Two"})
			},
			wantOriginalIDs: []string{"id-1"},
			wantCopiedIDs:   []string{"id-1", "id-2"},
		},
		{
			name: "deleting from original via KeepSelectedOnly does not affect copy",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
				{id: "id-2", displayName: "Resource Two"},
				{id: "id-3", displayName: "Resource Three"},
			},
			modifyOriginal: func(cache ResourceCache[*mockResource]) {
				cache.KeepSelectedOnly([]string{"id-1"})
			},
			modifyCopy:      func(cache ResourceCache[*mockResource]) {},
			wantOriginalIDs: []string{"id-1"},
			wantCopiedIDs:   []string{"id-1", "id-2", "id-3"},
		},
		{
			name: "deleting from copy via KeepSelectedOnly does not affect original",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
				{id: "id-2", displayName: "Resource Two"},
				{id: "id-3", displayName: "Resource Three"},
			},
			modifyOriginal: func(cache ResourceCache[*mockResource]) {},
			modifyCopy: func(cache ResourceCache[*mockResource]) {
				cache.KeepSelectedOnly([]string{"id-2"})
			},
			wantOriginalIDs: []string{"id-1", "id-2", "id-3"},
			wantCopiedIDs:   []string{"id-2"},
		},
		{
			name: "both caches modified independently",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Resource One"},
				{id: "id-2", displayName: "Resource Two"},
			},
			modifyOriginal: func(cache ResourceCache[*mockResource]) {
				cache.Set(&mockResource{id: "id-3", displayName: "Resource Three"})
				cache.KeepSelectedOnly([]string{"id-1", "id-3"})
			},
			modifyCopy: func(cache ResourceCache[*mockResource]) {
				cache.Set(&mockResource{id: "id-4", displayName: "Resource Four"})
				cache.KeepSelectedOnly([]string{"id-2", "id-4"})
			},
			wantOriginalIDs: []string{"id-1", "id-3"},
			wantCopiedIDs:   []string{"id-2", "id-4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewResourceCache[*mockResource]()
			cache.Store(tt.initialCache...)

			copiedCache := cache.Copy()

			// Apply modifications
			tt.modifyOriginal(cache)
			tt.modifyCopy(copiedCache)

			// Verify independence
			r.ElementsMatch(tt.wantOriginalIDs, cache.AllIDs(), "original cache IDs mismatch")
			r.ElementsMatch(tt.wantCopiedIDs, copiedCache.AllIDs(), "copied cache IDs mismatch")
		})
	}
}

func TestResourceCache_Copy_ModifyViaEitherCache(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	tests := []struct {
		name                 string
		initialCache         []*mockResource
		modifyViaCopiedCache bool
		resourceIDToModify   string
		newDisplayName       string
	}{
		{
			name: "modify resource via original cache, visible in copy",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Original Name"},
			},
			modifyViaCopiedCache: false,
			resourceIDToModify:   "id-1",
			newDisplayName:       "Changed via Original",
		},
		{
			name: "modify resource via copied cache, visible in original",
			initialCache: []*mockResource{
				{id: "id-1", displayName: "Original Name"},
			},
			modifyViaCopiedCache: true,
			resourceIDToModify:   "id-1",
			newDisplayName:       "Changed via Copy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewResourceCache[*mockResource]()
			cache.Store(tt.initialCache...)

			copiedCache := cache.Copy()

			// Modify resource via one of the caches
			var resourceToModify *mockResource
			if tt.modifyViaCopiedCache {
				resourceToModify = copiedCache.Get(tt.resourceIDToModify)
			} else {
				resourceToModify = cache.Get(tt.resourceIDToModify)
			}
			r.NotNil(resourceToModify)
			resourceToModify.displayName = tt.newDisplayName

			// Verify the change is visible in both caches (shallow copy behavior)
			resourceFromOriginal := cache.Get(tt.resourceIDToModify)
			resourceFromCopy := copiedCache.Get(tt.resourceIDToModify)

			r.Equal(tt.newDisplayName, resourceFromOriginal.GetDisplayName(),
				"change should be visible from original cache")
			r.Equal(tt.newDisplayName, resourceFromCopy.GetDisplayName(),
				"change should be visible from copied cache")

			// Verify both point to the same object
			r.Same(resourceFromOriginal, resourceFromCopy,
				"both caches should reference the same underlying object")
		})
	}
}
