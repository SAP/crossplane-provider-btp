package resources

import (
	"context"
	"errors"
	"testing"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/stretchr/testify/require"
)

func TestSelectCache_SelectAllKeepsEveryResource(t *testing.T) {
	t.Parallel()

	cache := NewResourceCache[*mockResource]()
	cache.Store(
		&mockResource{id: "first", displayName: "First"},
		&mockResource{id: "second", displayName: "Second"},
	)

	selectorAsked := false
	param := configparam.StringSlice("resource", "resource selector").
		WithPossibleValuesFn(func() ([]string, error) {
			selectorAsked = true
			return nil, errors.New("interactive selector must not be invoked")
		})

	err := SelectCache(context.Background(), cache, param, Options{SelectAll: true})

	require.NoError(t, err)
	require.False(t, selectorAsked)
	require.Equal(t, []string{"first", "second"}, cache.AllIDs())
}
