package scope

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

func TestSelect_FlagSelection(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		wantSubs  []string
		wantDirs  []string
		wantRoots int
	}{
		{
			name:      "single directory is one root expanding to its subtree",
			flag:      "dir-1",
			wantSubs:  []string{"sa-b", "sa-c", "sa-d"},
			wantDirs:  []string{"dir-1", "dir-1-1"},
			wantRoots: 1,
		},
		{
			name:      "single subaccount is one root",
			flag:      "sa-a",
			wantSubs:  []string{"sa-a"},
			wantDirs:  []string{},
			wantRoots: 1,
		},
		{
			name:      "two explicit roots count as two",
			flag:      "sa-a,dir-1-1",
			wantSubs:  []string{"sa-a", "sa-c", "sa-d"},
			wantDirs:  []string{"dir-1-1"},
			wantRoots: 2,
		},
		{
			name:      "regex over display name resolves",
			flag:      "^Subaccount-A$",
			wantSubs:  []string{"sa-a"},
			wantDirs:  []string{},
			wantRoots: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			Reset()
			t.Cleanup(func() {
				viper.Reset()
				Reset()
			})

			command := &cobra.Command{}
			selectorParam.AttachToCommand(command)
			require.NoError(t, command.ParseFlags([]string{"--subaccount", tt.flag}))
			selectorParam.BindConfiguration(command)

			sel, err := Select(context.Background(), btpcli.NewClient(fakeHierarchyCLI(t)))
			require.NoError(t, err)

			require.ElementsMatch(t, tt.wantSubs, sel.SubaccountIDs())
			require.ElementsMatch(t, tt.wantDirs, sel.DirectoryIDs())

			roots, err := RootCount(context.Background(), btpcli.NewClient(fakeHierarchyCLI(t)))
			require.NoError(t, err)
			require.Equal(t, tt.wantRoots, roots)
		})
	}
}

func fakeHierarchyCLI(t *testing.T) string {
	t.Helper()

	const hierarchy = `{"guid":"ga","subaccounts":[{"guid":"sa-a","displayName":"Subaccount-A"}],"children":[{"guid":"dir-1","displayName":"Dir-1","directoryType":"FOLDER","subaccounts":[{"guid":"sa-b","displayName":"Subaccount-B"}],"children":[{"guid":"dir-1-1","displayName":"Dir-1_1","directoryType":"FOLDER","subaccounts":[{"guid":"sa-c","displayName":"Subaccount-C"},{"guid":"sa-d","displayName":"Subaccount-D"}]}]}]}`

	cliPath := filepath.Join(t.TempDir(), "btp")
	script := `#!/bin/sh
printf '%s\n' '` + hierarchy + `'
`
	require.NoError(t, os.WriteFile(cliPath, []byte(script), 0o755))
	return cliPath
}
