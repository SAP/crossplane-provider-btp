package directoryentitlement

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestDirectoryEntitlementSelection(t *testing.T) {
	viper.Reset()
	t.Cleanup(func() {
		viper.Reset()
		entitlementCache = nil
	})
	viper.Set("directory", []string{"dir-one", "dir-two"})
	client := btpcli.NewClient(fakeBTPCLI(t))
	for _, tt := range []struct {
		name      string
		selector  []string
		selectAll bool
		wantIDs   []string
	}{
		{name: "all directory plans, not child assignments", selectAll: true, wantIDs: []string{"dir-one/service/standard", "dir-two/service/standard"}},
		{name: "regex across directories", selector: []string{"^service/standard$"}, wantIDs: []string{"dir-one/service/standard", "dir-two/service/standard"}},
		{name: "compound ID", selector: []string{"dir-two/service/standard"}, wantIDs: []string{"dir-two/service/standard"}},
		{name: "interactive display", selector: []string{"service/standard - [dir-one/service/standard]"}, wantIDs: []string{"dir-one/service/standard"}},
		{name: "child assignment excluded", selector: []string{"child-service"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entitlementCache = nil
			viper.Set(KindName, tt.selector)
			cache, err := Get(t.Context(), client, resources.Options{SelectAll: tt.selectAll})
			require.NoError(t, err)
			require.Equal(t, tt.wantIDs, cache.AllIDs())
			for _, e := range cache.All() {
				require.Equal(t, "plan-id", e.plan.UniqueIdentifier)
				require.Equal(t, 12.0, *e.plan.Amount)
				require.True(t, e.plan.AutoAssign)
				require.Equal(t, 2.0, *e.plan.AutoDistributeAmount)
			}
			cached, err := Get(t.Context(), btpcli.NewClient("nonexistent-btp"), resources.Options{})
			require.NoError(t, err)
			require.Same(t, cache, cached)
		})
	}
}

func TestDirectoryEntitlementSourceErrors(t *testing.T) {
	viper.Reset()
	t.Cleanup(func() {
		viper.Reset()
		entitlementCache = nil
	})
	for _, tt := range []struct {
		name        string
		directories []string
		wantError   string
	}{
		{name: "missing source", wantError: "--directory is required"},
		{name: "empty source", directories: []string{" "}, wantError: "non-empty directory GUIDs"},
		{name: "CLI failure", directories: []string{"dir-one"}, wantError: "failed to list entitlements for directory dir-one"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entitlementCache = nil
			viper.Set("directory", tt.directories)
			_, err := Get(t.Context(), btpcli.NewClient("nonexistent-btp"), resources.Options{SelectAll: true})
			require.ErrorContains(t, err, tt.wantError)
		})
	}
}

func fakeBTPCLI(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "btp")
	require.NoError(t, os.WriteFile(path, []byte(`#!/bin/sh
case "$*" in
  "--format json list accounts/entitlement --directory dir-one"|"--format json list accounts/entitlement --directory dir-two")
    cat <<'JSON'
{
  "entitledServices": [{"name":"service", "servicePlans":[{
    "name":"standard", "uniqueIdentifier":"plan-id", "category":"SERVICE",
    "amount":12, "autoAssign":true, "autoDistributeAmount":2
  }]}],
  "assignedServices": [{"name":"child-service", "servicePlans":[{
    "name":"standard", "assignmentInfo":[{"entityId":"child-subaccount", "amount":2}]
  }]}]
}
JSON
    ;;
  *) echo "unexpected command: $*" >&2; exit 1 ;;
esac
`), 0o755))
	return path
}
