package subaccount

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/scope"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestGet_UsesNonInteractiveScopeSelector(t *testing.T) {
	tests := []struct {
		name      string
		configure func(t *testing.T)
		wantIDs   []string
	}{
		{
			name: "flag selects by id and regex",
			configure: func(t *testing.T) {
				command := &cobra.Command{}
				scope.Param().AttachToCommand(command)
				require.NoError(t, command.ParseFlags([]string{"--subaccount", "sa-a,^Subaccount-C$"}))
				scope.Param().BindConfiguration(command)
			},
			wantIDs: []string{"sa-a", "sa-c"},
		},
		{
			name: "selecting a directory expands to descendant subaccounts",
			configure: func(t *testing.T) {
				command := &cobra.Command{}
				scope.Param().AttachToCommand(command)
				require.NoError(t, command.ParseFlags([]string{"--subaccount", "dir-1"}))
				scope.Param().BindConfiguration(command)
			},
			wantIDs: []string{"sa-b", "sa-c", "sa-d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			subaccountCache = nil
			scope.Reset()
			t.Cleanup(func() {
				viper.Reset()
				subaccountCache = nil
				scope.Reset()
			})
			tt.configure(t)

			cache, err := Get(context.Background(), btpcli.NewClient(fakeBTPCLI(t)))

			require.NoError(t, err)
			require.ElementsMatch(t, tt.wantIDs, cache.AllIDs())
		})
	}
}

func TestGet_EnrichesLabelsFromFlatList(t *testing.T) {
	viper.Reset()
	subaccountCache = nil
	scope.Reset()
	t.Cleanup(func() {
		viper.Reset()
		subaccountCache = nil
		scope.Reset()
	})

	command := &cobra.Command{}
	scope.Param().AttachToCommand(command)
	require.NoError(t, command.ParseFlags([]string{"--subaccount", "sa-a"}))
	scope.Param().BindConfiguration(command)

	cache, err := Get(context.Background(), btpcli.NewClient(fakeBTPCLI(t)))
	require.NoError(t, err)

	sa := cache.Get("sa-a")
	require.NotNil(t, sa)
	require.Equal(t, map[string][]string{"env": {"prod"}}, sa.Labels)
}

// fakeBTPCLI writes a stub btp CLI that answers the two commands Get relies on:
//   - get accounts/global-account --show-hierarchy (scope selection)
//   - list accounts/subaccount --authorized         (label enrichment)
//
// The hierarchy models:
//
//	GA
//	└ Subaccount-A            (sa-a)
//	└ Dir-1                   (dir-1)
//	  └ Subaccount-B          (sa-b)
//	  └ Dir-1_1               (dir-1-1)
//	    └ Subaccount-C        (sa-c)
//	    └ Subaccount-D        (sa-d)
func fakeBTPCLI(t *testing.T) string {
	t.Helper()

	const hierarchy = `{"guid":"ga","subaccounts":[{"guid":"sa-a","displayName":"Subaccount-A"}],"children":[{"guid":"dir-1","displayName":"Dir-1","directoryType":"FOLDER","subaccounts":[{"guid":"sa-b","displayName":"Subaccount-B"}],"children":[{"guid":"dir-1-1","displayName":"Dir-1_1","directoryType":"FOLDER","subaccounts":[{"guid":"sa-c","displayName":"Subaccount-C"},{"guid":"sa-d","displayName":"Subaccount-D"}]}]}]}`
	const flatList = `{"value":[{"guid":"sa-a","displayName":"Subaccount-A","labels":{"env":["prod"]}},{"guid":"sa-b","displayName":"Subaccount-B"},{"guid":"sa-c","displayName":"Subaccount-C"},{"guid":"sa-d","displayName":"Subaccount-D"}]}`

	cliPath := filepath.Join(t.TempDir(), "btp")
	script := `#!/bin/sh
for arg in "$@"; do
  case "$arg" in
    accounts/global-account) printf '%s\n' '` + hierarchy + `'; exit 0 ;;
    accounts/subaccount) printf '%s\n' '` + flatList + `'; exit 0 ;;
  esac
done
printf '%s\n' '{}'
`
	require.NoError(t, os.WriteFile(cliPath, []byte(script), 0o755))
	return cliPath
}
