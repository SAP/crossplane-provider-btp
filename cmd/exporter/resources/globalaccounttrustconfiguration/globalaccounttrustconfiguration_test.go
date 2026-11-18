package globalaccounttrustconfiguration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"
	xpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestTrustConfigurationSelection(t *testing.T) {
	viper.Reset()
	t.Cleanup(func() {
		viper.Reset()
		trustCache = nil
	})
	client := btpcli.NewClient(fakeBTPCLI(t))
	for _, tt := range []struct {
		name      string
		selector  []string
		selectAll bool
		wantIDs   []string
	}{
		{name: "all excludes the built-in platform trust", selectAll: true, wantIDs: []string{"custom", "other"}},
		{name: "origin regex", selector: []string{"^custom$"}, wantIDs: []string{"custom"}},
		{name: "exact origin", selector: []string{"other"}, wantIDs: []string{"other"}},
		{name: "interactive display value", selector: []string{"custom - [custom]"}, wantIDs: []string{"custom"}},
		{name: "platform trust is excluded", selector: []string{"sap.default"}},
		{name: "no match", selector: []string{"missing"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			trustCache = nil
			viper.Set(KindName, tt.selector)
			cache, err := Get(t.Context(), client, resources.Options{SelectAll: tt.selectAll})
			require.NoError(t, err)
			require.Equal(t, tt.wantIDs, cache.AllIDs())
			cached, err := Get(t.Context(), btpcli.NewClient("nonexistent-btp"), resources.Options{})
			require.NoError(t, err)
			require.Same(t, cache, cached)
		})
	}

	trustCache = nil
	viper.Set(KindName, []string{".*"})
	handler := &recordingHandler{}
	require.NoError(t, exporter{}.Export(t.Context(), client, handler, resources.Options{}))
	require.Empty(t, handler.warnings)
	require.Len(t, handler.resources, 2)
	names := map[string]bool{}
	for _, cr := range handler.resources {
		require.False(t, names[cr.Name])
		names[cr.Name] = true
	}

	trustCache = nil
	viper.Set(KindName, []string{"missing"})
	handler = &recordingHandler{}
	require.NoError(t, exporter{}.Export(t.Context(), client, handler, resources.Options{}))
	require.Empty(t, handler.resources)
	require.Len(t, handler.warnings, 1)
	require.ErrorContains(t, handler.warnings[0], "no global account trust configurations found")
}

func fakeBTPCLI(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "btp")
	require.NoError(t, os.WriteFile(path, []byte(`#!/bin/sh
case "$*" in
  "--format json list security/trust")
    cat <<'JSON'
[
  {"originKey":"custom", "name":"Custom trust", "identityProvider":"tenant.example.com",
   "availableForUserLogon":"false", "createShadowUsersDuringLogon":"true"},
  {"originKey":"other", "name":"Other trust", "identityProvider":"other.example.com",
   "availableForUserLogon":"true", "createShadowUsersDuringLogon":"true"},
  {"originKey":"sap.default", "name":"sap.default", "identityProvider":null,
   "availableForUserLogon":"true", "createShadowUsersDuringLogon":"false"}
]
JSON
    ;;
  *) echo "unexpected command: $*" >&2; exit 1 ;;
esac
`), 0o755))
	return path
}

type recordingHandler struct {
	export.EventHandler
	resources []*v1alpha1.GlobalaccountTrustConfiguration
	warnings  []error
}

func (h *recordingHandler) Resource(value xpresource.Object) {
	wrapped := value.(*yaml.ResourceWithComment)
	h.resources = append(h.resources, wrapped.Resource().(*v1alpha1.GlobalaccountTrustConfiguration))
}

func (h *recordingHandler) Warn(err error) { h.warnings = append(h.warnings, err) }
