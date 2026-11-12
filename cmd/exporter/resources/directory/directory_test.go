package directory

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

func TestGet_SelectsDirectoriesViaScope(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		wantIDs   []string
	}{
		{
			name:      "flag selects a single directory by id",
			flagValue: "dir-1-1",
			wantIDs:   []string{"dir-1-1"},
		},
		{
			name:      "selecting a parent directory expands to descendant directories",
			flagValue: "dir-1",
			wantIDs:   []string{"dir-1", "dir-1-1"},
		},
		{
			name:      "selecting a subaccount yields no directories",
			flagValue: "sa-a",
			wantIDs:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			directoryCache = nil
			scope.Reset()
			t.Cleanup(func() {
				viper.Reset()
				directoryCache = nil
				scope.Reset()
			})

			command := &cobra.Command{}
			scope.Param().AttachToCommand(command)
			require.NoError(t, command.ParseFlags([]string{"--subaccount", tt.flagValue}))
			scope.Param().BindConfiguration(command)

			cache, err := Get(context.Background(), btpcli.NewClient(fakeBTPCLI(t)))

			require.NoError(t, err)
			require.ElementsMatch(t, tt.wantIDs, cache.AllIDs())
		})
	}
}

func TestGetK8sResourceNameByID(t *testing.T) {
	viper.Reset()
	directoryCache = nil
	scope.Reset()
	t.Cleanup(func() {
		viper.Reset()
		directoryCache = nil
		scope.Reset()
	})

	command := &cobra.Command{}
	scope.Param().AttachToCommand(command)
	require.NoError(t, command.ParseFlags([]string{"--subaccount", "dir-1"}))
	scope.Param().BindConfiguration(command)

	client := btpcli.NewClient(fakeBTPCLI(t))

	t.Run("resolves a selected directory to its k8s resource name", func(t *testing.T) {
		name, err := GetK8sResourceNameByID(context.Background(), client, "dir-1")
		require.NoError(t, err)
		require.Equal(t, "dir-1", name)
	})

	t.Run("empty id returns an error", func(t *testing.T) {
		_, err := GetK8sResourceNameByID(context.Background(), client, "")
		require.Error(t, err)
	})

	t.Run("unknown id returns an error", func(t *testing.T) {
		_, err := GetK8sResourceNameByID(context.Background(), client, "does-not-exist")
		require.Error(t, err)
	})
}

// fakeBTPCLI writes a stub btp CLI that answers the command Get relies on:
//   - get accounts/global-account --show-hierarchy (scope selection)
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
	const flatList = `{"value":[{"guid":"sa-a","displayName":"Subaccount-A"},{"guid":"sa-b","displayName":"Subaccount-B"},{"guid":"sa-c","displayName":"Subaccount-C"},{"guid":"sa-d","displayName":"Subaccount-D"}]}`

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
