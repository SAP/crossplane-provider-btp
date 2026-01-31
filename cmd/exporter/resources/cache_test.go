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
			name: "no matches",
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
