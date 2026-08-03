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

func TestGet_UsesNonInteractiveSubaccountSelector(t *testing.T) {
	tests := []struct {
		name      string
		configure func(t *testing.T)
	}{
		{
			name: "flag",
			configure: func(t *testing.T) {
				command := &cobra.Command{}
				subaccountParam.AttachToCommand(command)
				require.NoError(t, command.ParseFlags([]string{"--subaccount", "first,^Third$"}))
				subaccountParam.BindConfiguration(command)
			},
		},
		{
			name: "configuration",
			configure: func(t *testing.T) {
				configPath := filepath.Join(t.TempDir(), "exporter.yaml")
				require.NoError(t, os.WriteFile(configPath, []byte("subaccount:\n  - first\n  - ^Third$\n"), 0o600))
				viper.SetConfigFile(configPath)
				require.NoError(t, viper.ReadInConfig())
			},
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
			tt.configure(t)

			cache, err := Get(context.Background(), btpcli.NewClient(fakeBTPCLI(t)))

			require.NoError(t, err)
			require.Equal(t, []string{"first", "third"}, cache.AllIDs())
		})
	}
}

func fakeBTPCLI(t *testing.T) string {
	t.Helper()

	cliPath := filepath.Join(t.TempDir(), "btp")
	require.NoError(t, os.WriteFile(cliPath, []byte(`#!/bin/sh
printf '%s\n' '{"value":[{"guid":"first","displayName":"First"},{"guid":"second","displayName":"Second"},{"guid":"third","displayName":"Third"}]}'
`), 0o755))
	return cliPath
}
