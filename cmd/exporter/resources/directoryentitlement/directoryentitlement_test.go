package directoryentitlement

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SAP/xp-clifford/yaml"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/directory"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/hierarchy"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestDirectoryEntitlementSelection(t *testing.T) {
	resetCaches(t)
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
	resetCaches(t)
	for _, tt := range []struct {
		name        string
		directories []string
		wantError   string
	}{
		{name: "CLI failure", directories: []string{"dir-one"}, wantError: "failed to get directories"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entitlementCache = nil
			viper.Set("directory", tt.directories)
			_, err := Get(t.Context(), btpcli.NewClient("nonexistent-btp"), resources.Options{SelectAll: true})
			require.ErrorContains(t, err, tt.wantError)
		})
	}
}

func resetCaches(t *testing.T) {
	t.Helper()
	reset := func() {
		viper.Reset()
		entitlementCache = nil
		directory.Reset()
		subaccount.Reset()
		hierarchy.Reset()
	}
	reset()
	viper.Set("subaccount", []string{"sa-one"})
	t.Cleanup(reset)
}

type stubEventHandler struct {
	warnings []error
	objects  []resource.Object
}

func (s *stubEventHandler) Warn(err error)               { s.warnings = append(s.warnings, err) }
func (s *stubEventHandler) Resource(obj resource.Object) { s.objects = append(s.objects, obj) }
func (s *stubEventHandler) Stop()                        {}

func TestDirectoryEntitlementDirectorySelectionAndReferences(t *testing.T) {
	for _, tt := range []struct {
		name     string
		selector []string
		resolve  bool
		wantIDs  []string
	}{
		{name: "derived ancestors", wantIDs: []string{"dir-one", "dir-two"}},
		{name: "directory name", selector: []string{"^Directory One$"}, resolve: true, wantIDs: []string{"dir-one"}},
		{name: "directory ID", selector: []string{"dir-one"}, resolve: true, wantIDs: []string{"dir-one"}},
		{name: "no matching directory", selector: []string{"unknown"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resetCaches(t)
			viper.Set("directory", tt.selector)
			handler := &stubEventHandler{}
			err := (exporter{}).Export(t.Context(), btpcli.NewClient(fakeBTPCLI(t)), handler, resources.Options{SelectAll: true, ResolveReferences: tt.resolve})
			require.NoError(t, err)
			require.Len(t, handler.objects, len(tt.wantIDs))
			if len(tt.wantIDs) == 0 {
				require.Len(t, handler.warnings, 1)
				return
			}
			require.Empty(t, handler.warnings)
			var gotIDs []string
			for _, obj := range handler.objects {
				cr := obj.(*yaml.ResourceWithComment).Resource().(*v1alpha1.DirectoryEntitlement)
				id := strings.SplitN(cr.Annotations["crossplane.io/external-name"], "/", 2)[0]
				gotIDs = append(gotIDs, id)
				require.Equal(t, id+"/service/standard", cr.Annotations["crossplane.io/external-name"])
				if tt.resolve {
					require.Nil(t, cr.Spec.ForProvider.DirectoryID)
					require.Equal(t, &xpv1.Reference{Name: "directory-one.dir-one"}, cr.Spec.ForProvider.DirectoryRef)
				} else {
					require.Equal(t, id, *cr.Spec.ForProvider.DirectoryID)
					require.Nil(t, cr.Spec.ForProvider.DirectoryRef)
				}
			}
			require.ElementsMatch(t, tt.wantIDs, gotIDs)
		})
	}
}

func TestDirectoryEntitlementReferenceFailure(t *testing.T) {
	resetCaches(t)
	client := btpcli.NewClient(fakeBTPCLI(t))
	handler := &stubEventHandler{}
	e := &directoryEntitlement{directoryID: "unknown", serviceName: "service", plan: btpcli.DirectoryEntitledServicePlan{Name: "standard"}, ResourceWithComment: yaml.NewResourceWithComment(nil)}
	result := convertDirectoryEntitlementResource(t.Context(), client, e, handler, true)
	cr := result.Resource().(*v1alpha1.DirectoryEntitlement)
	require.Equal(t, "unknown", *cr.Spec.ForProvider.DirectoryID)
	require.Nil(t, cr.Spec.ForProvider.DirectoryRef)
	require.Len(t, handler.warnings, 1)
	comment, _ := result.Comment()
	require.Contains(t, comment, resources.WarnCannotResolveDirectory)
}

func TestDirectoryEntitlementSkipsDirectoriesWithoutEntitlementManagement(t *testing.T) {
	for _, tt := range []struct {
		name     string
		features string
	}{
		{name: "plain directory", features: `,"directoryFeatures":["DEFAULT"]`},
		{name: "missing features"},
		{name: "empty features", features: `,"directoryFeatures":[]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resetCaches(t)
			client := btpcli.NewClient(fakeBTPCLIWithHierarchy(t, `{"guid":"ga","children":[{"guid":"dir-one","displayName":"Directory One","directoryFeatures":["DEFAULT","ENTITLEMENTS"],"children":[{"guid":"plain-dir","displayName":"Plain Directory"`+tt.features+`,"subaccounts":[{"guid":"sa-one"}]}]}]}`))
			// The fake CLI rejects entitlement queries for plain-dir, so this
			// also verifies that unsupported directories are never queried.
			handler := &stubEventHandler{}
			err := (exporter{}).Export(t.Context(), client, handler, resources.Options{SelectAll: true})
			require.NoError(t, err)
			require.Empty(t, handler.warnings)
			require.Len(t, handler.objects, 1)
			require.Equal(t, "dir-one/service/standard", handler.objects[0].GetAnnotations()["crossplane.io/external-name"])
		})
	}
}

func fakeBTPCLI(t *testing.T) string {
	t.Helper()
	return fakeBTPCLIWithHierarchy(t, `{"guid":"ga","children":[{"guid":"dir-one","displayName":"Directory One","directoryFeatures":["DEFAULT","ENTITLEMENTS"],"children":[{"guid":"dir-two","displayName":"Directory Two","directoryFeatures":["DEFAULT","ENTITLEMENTS"],"subaccounts":[{"guid":"sa-one"}]}]}]}`)
}

func fakeBTPCLIWithHierarchy(t *testing.T, hierarchyJSON string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "btp")
	require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(`#!/bin/sh
case "$*" in
  "--format json list accounts/subaccount --authorized")
    echo '{"value":[{"guid":"sa-one","displayName":"Subaccount One"}]}' ;;
  "--format json get accounts/global-account --show-hierarchy")
    echo 'HIERARCHY_JSON' ;;
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
`, "HIERARCHY_JSON", hierarchyJSON)), 0o755))
	return path
}
