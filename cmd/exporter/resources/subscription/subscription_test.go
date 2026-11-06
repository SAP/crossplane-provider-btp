package subscription

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionSelection(t *testing.T) {
	viper.Reset()
	t.Cleanup(func() {
		viper.Reset()
		subscriptionCache = nil
	})
	viper.Set("subaccount", []string{".*"})
	btpClient := btpcli.NewClient(fakeBTPCLI(t))

	for _, tt := range []struct {
		name      string
		selector  []string
		selectAll bool
		wantIDs   []string
	}{
		{name: "all retains existing subscriptions across subaccounts", selectAll: true, wantIDs: []string{"guid-sa-one", "guid-sa-two", "sa-one/my-app/", "sa-two/my-app/"}},
		{name: "regex selects plan across subaccounts", selector: []string{"^my-app/standard$"}, wantIDs: []string{"guid-sa-one", "guid-sa-two"}},
		{name: "scoped fallback without GUID", selector: []string{"sa-two/my-app/"}, wantIDs: []string{"sa-two/my-app/"}},
		{name: "GUID selects single subscription", selector: []string{"guid-sa-one"}, wantIDs: []string{"guid-sa-one"}},
		{name: "scoped key does not select subscription with GUID", selector: []string{"sa-one/my-app/standard"}},
		{name: "interactive display value uses GUID", selector: []string{"my-app/standard - [guid-sa-two]"}, wantIDs: []string{"guid-sa-two"}},
		{name: "unsubscribed applications excluded", selector: []string{"available-app"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			subscriptionCache = nil
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
  "--format json list accounts/subscription --subaccount sa-one"|"--format json list accounts/subscription --subaccount sa-two")
    subaccount_id=$6
    cat <<JSON
{
  "applications": [
    {"appName":"my-app", "planName":"standard", "state":"SUBSCRIBED", "subscriptionGUID":"guid-$subaccount_id"},
    {"appName":"my-app", "planName":"", "state":"SUBSCRIBE_FAILED"},
    {"appName":"available-app", "planName":"free", "state":"NOT_SUBSCRIBED"}
  ]
}
JSON
    ;;
  *) echo "unexpected command: $*" >&2; exit 1 ;;
esac
`), 0o755))
	return cliPath
}
