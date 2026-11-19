package rolecollection

import (
	"testing"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestRoleCollectionSelection(t *testing.T) {
	viper.Reset()
	t.Cleanup(func() {
		viper.Reset()
		roleCollectionCache = nil
		subaccount.Reset()
	})
	viper.Set("subaccount", []string{".*"})
	btpClient := btpcli.NewClient(fakeBTPCLI(t))

	for _, tt := range []struct {
		name      string
		selector  []string
		selectAll bool
		wantIDs   []string
	}{
		{
			name:      "all retains every role collection across subaccounts",
			selectAll: true,
			wantIDs:   []string{"sa-one/alpha", "sa-one/beta", "sa-two/alpha", "sa-two/gamma"},
		},
		{
			name:     "regex on name matches across subaccounts",
			selector: []string{"^alpha$"},
			wantIDs:  []string{"sa-one/alpha", "sa-two/alpha"},
		},
		{
			name:     "scoped id selects a single role collection",
			selector: []string{"sa-one/beta"},
			wantIDs:  []string{"sa-one/beta"},
		},
		{
			name:     "display value selects by name and parent",
			selector: []string{"gamma - [sa-two/gamma]"},
			wantIDs:  []string{"sa-two/gamma"},
		},
		{
			name:     "non matching selector keeps nothing",
			selector: []string{"does-not-exist"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			roleCollectionCache = nil
			subaccount.Reset()
			viper.Set(KindName, tt.selector)

			cache, err := Get(t.Context(), btpClient, resources.Options{SelectAll: tt.selectAll})
			require.NoError(t, err)
			require.Equal(t, tt.wantIDs, cache.AllIDs())

			// Reusing the selection must not call the CLI again.
			cached, err := Get(t.Context(), btpcli.NewClient("nonexistent-btp"), resources.Options{})
			require.NoError(t, err)
			require.Same(t, cache, cached)
		})
	}
}
