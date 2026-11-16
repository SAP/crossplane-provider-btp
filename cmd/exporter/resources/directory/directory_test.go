package directory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/hierarchy"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestGet_DerivesAncestorDirectoriesFromSelectedSubaccounts(t *testing.T) {
	tests := []struct {
		name         string
		flagValue    string
		directoryVal string
		wantIDs      []string
	}{
		{
			name:      "subaccount under the global account has no ancestor directories",
			flagValue: "sa-a",
			wantIDs:   []string{},
		},
		{
			name:      "subaccount one level deep yields its single ancestor directory",
			flagValue: "sa-b",
			wantIDs:   []string{"dir-1"},
		},
		{
			name:      "subaccount two levels deep yields the full ancestor chain",
			flagValue: "sa-c",
			wantIDs:   []string{"dir-1", "dir-1-1"},
		},
		{
			name:      "multiple subaccounts contribute a deduplicated ancestor set",
			flagValue: "sa-b,sa-c",
			wantIDs:   []string{"dir-1", "dir-1-1"},
		},
		{
			name:         "directory selector narrows the derived ancestors by GUID",
			flagValue:    "sa-c",
			directoryVal: "dir-1",
			wantIDs:      []string{"dir-1"},
		},
		{
			name:         "directory selector matches by display-name regex",
			flagValue:    "sa-c",
			directoryVal: "Dir-1_1",
			wantIDs:      []string{"dir-1-1"},
		},
		{
			name:         "directory selector with no match yields an empty set",
			flagValue:    "sa-c",
			directoryVal: "no-such-directory",
			wantIDs:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetCaches(t)

			command := &cobra.Command{}
			subaccount.Param().AttachToCommand(command)
			Param().AttachToCommand(command)
			args := []string{"--subaccount", tt.flagValue}
			if tt.directoryVal != "" {
				args = append(args, "--directory", tt.directoryVal)
			}
			require.NoError(t, command.ParseFlags(args))
			subaccount.Param().BindConfiguration(command)
			Param().BindConfiguration(command)

			cache, err := Get(context.Background(), btpcli.NewClient(fakeBTPCLI(t)))

			require.NoError(t, err)
			require.ElementsMatch(t, tt.wantIDs, cache.AllIDs())
		})
	}
}

func TestGetK8sResourceNameByID(t *testing.T) {
	resetCaches(t)

	command := &cobra.Command{}
	subaccount.Param().AttachToCommand(command)
	require.NoError(t, command.ParseFlags([]string{"--subaccount", "sa-c"}))
	subaccount.Param().BindConfiguration(command)

	client := btpcli.NewClient(fakeBTPCLI(t))

	t.Run("resolves a derived directory to its k8s resource name", func(t *testing.T) {
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

func resetCaches(t *testing.T) {
	t.Helper()
	viper.Reset()
	directoryCache = nil
	subaccount.Reset()
	hierarchy.Reset()
	t.Cleanup(func() {
		viper.Reset()
		directoryCache = nil
		subaccount.Reset()
		hierarchy.Reset()
	})
}

// fakeBTPCLI writes a stub btp CLI that answers the two commands Get relies on:
//   - list accounts/subaccount --authorized         (subaccount selection)
//   - get accounts/global-account --show-hierarchy  (ancestor derivation)
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

	const hierarchyJSON = `{"guid":"ga","subaccounts":[{"guid":"sa-a","displayName":"Subaccount-A"}],"children":[{"guid":"dir-1","displayName":"Dir-1","directoryType":"FOLDER","subaccounts":[{"guid":"sa-b","displayName":"Subaccount-B"}],"children":[{"guid":"dir-1-1","displayName":"Dir-1_1","directoryType":"FOLDER","subaccounts":[{"guid":"sa-c","displayName":"Subaccount-C"},{"guid":"sa-d","displayName":"Subaccount-D"}]}]}]}`
	const flatList = `{"value":[{"guid":"sa-a","displayName":"Subaccount-A"},{"guid":"sa-b","displayName":"Subaccount-B"},{"guid":"sa-c","displayName":"Subaccount-C"},{"guid":"sa-d","displayName":"Subaccount-D"}]}`

	cliPath := filepath.Join(t.TempDir(), "btp")
	script := `#!/bin/sh
for arg in "$@"; do
  case "$arg" in
    accounts/global-account) printf '%s\n' '` + hierarchyJSON + `'; exit 0 ;;
    accounts/subaccount) printf '%s\n' '` + flatList + `'; exit 0 ;;
  esac
done
printf '%s\n' '{}'
`
	require.NoError(t, os.WriteFile(cliPath, []byte(script), 0o755))
	return cliPath
}
