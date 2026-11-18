package subaccount

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestGet_SelectsAuthorizedSubaccounts(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		wantIDs   []string
	}{
		{
			name:      "flag selects by id and regex",
			flagValue: "sa-a,^Subaccount-C$",
			wantIDs:   []string{"sa-a", "sa-c"},
		},
		{
			name:      "flag selects a single subaccount by id",
			flagValue: "sa-b",
			wantIDs:   []string{"sa-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			subaccountCache = nil
			t.Cleanup(func() {
				viper.Reset()
				subaccountCache = nil
			})

			command := &cobra.Command{}
			subaccountParam.AttachToCommand(command)
			require.NoError(t, command.ParseFlags([]string{"--subaccount", tt.flagValue}))
			subaccountParam.BindConfiguration(command)

			cache, err := Get(context.Background(), btpcli.NewClient(fakeBTPCLI(t)))

			require.NoError(t, err)
			require.ElementsMatch(t, tt.wantIDs, cache.AllIDs())
		})
	}
}

func TestGet_EnrichesLabelsFromAuthorizedList(t *testing.T) {
	viper.Reset()
	subaccountCache = nil
	t.Cleanup(func() {
		viper.Reset()
		subaccountCache = nil
	})

	command := &cobra.Command{}
	subaccountParam.AttachToCommand(command)
	require.NoError(t, command.ParseFlags([]string{"--subaccount", "sa-a"}))
	subaccountParam.BindConfiguration(command)

	cache, err := Get(context.Background(), btpcli.NewClient(fakeBTPCLI(t)))
	require.NoError(t, err)

	sa := cache.Get("sa-a")
	require.NotNil(t, sa)
	require.Equal(t, map[string][]string{"env": {"prod"}}, sa.Labels)
}

// fakeBTPCLI writes a stub btp CLI that answers the command Get relies on:
//   - list accounts/subaccount --authorized (the authorized subaccount list)
func fakeBTPCLI(t *testing.T) string {
	t.Helper()

	const flatList = `{"value":[{"guid":"sa-a","displayName":"Subaccount-A","labels":{"env":["prod"]}},{"guid":"sa-b","displayName":"Subaccount-B"},{"guid":"sa-c","displayName":"Subaccount-C"},{"guid":"sa-d","displayName":"Subaccount-D"}]}`

	cliPath := filepath.Join(t.TempDir(), "btp")
	script := `#!/bin/sh
for arg in "$@"; do
  case "$arg" in
    accounts/subaccount) printf '%s\n' '` + flatList + `'; exit 0 ;;
  esac
done
printf '%s\n' '{}'
`
	require.NoError(t, os.WriteFile(cliPath, []byte(script), 0o755))
	return cliPath
}
