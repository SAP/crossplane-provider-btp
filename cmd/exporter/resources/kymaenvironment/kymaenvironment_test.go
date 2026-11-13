package kymaenvironment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestKymaEnvironmentSelection(t *testing.T) {
	viper.Reset()
	t.Cleanup(func() {
		viper.Reset()
		cfEnvCache = nil
	})
	viper.Set("subaccount", []string{".*"})
	btpClient := btpcli.NewClient(fakeBTPCLI(t))

	for _, tt := range []struct {
		name      string
		selector  []string
		selectAll bool
		wantIDs   []string
	}{
		{name: "all retains kyma environments across subaccounts", selectAll: true, wantIDs: []string{"kyma-sa-one", "kyma-sa-two"}},
		{name: "regex selects environment by name", selector: []string{"^my-kyma$"}, wantIDs: []string{"kyma-sa-one", "kyma-sa-two"}},
		{name: "ID selects single environment", selector: []string{"kyma-sa-one"}, wantIDs: []string{"kyma-sa-one"}},
		{name: "interactive display value uses ID", selector: []string{"my-kyma - [kyma-sa-two]"}, wantIDs: []string{"kyma-sa-two"}},
		{name: "non-kyma environments excluded", selector: []string{"my-cf"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfEnvCache = nil
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

func fakeBTPCLI(t *testing.T) string {
	t.Helper()
	cliPath := filepath.Join(t.TempDir(), "btp")
	require.NoError(t, os.WriteFile(cliPath, []byte(`#!/bin/sh
case "$*" in
  "--format json list accounts/subaccount --authorized")
    echo '{"value":[{"guid":"sa-one","displayName":"First"},{"guid":"sa-two","displayName":"Second"}]}'
    ;;
  "--format json list accounts/environment-instance --subaccount sa-one"|"--format json list accounts/environment-instance --subaccount sa-two")
    subaccount_id=$6
    cat <<JSON
{
  "environmentInstances": [
    {"id":"kyma-$subaccount_id", "name":"my-kyma", "environmentType":"kyma", "planName":"aws", "subaccountGUID":"$subaccount_id"},
    {"id":"cf-$subaccount_id", "name":"my-cf", "environmentType":"cloudfoundry", "planName":"standard", "subaccountGUID":"$subaccount_id"}
  ]
}
JSON
    ;;
  *) echo "unexpected command: $*" >&2; exit 1 ;;
esac
`), 0o755))
	return cliPath
}
