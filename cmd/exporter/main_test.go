package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	xpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestExportCmdSelectiveInteractiveExportSelectsSubaccountBeforeKindsAndChildResources(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	originalGetSelectedSubaccountCount := getSelectedSubaccountCount
	originalSelectResourceKinds := selectResourceKinds
	originalResourceExportFn := resourceExportFn
	t.Cleanup(func() {
		getSelectedSubaccountCount = originalGetSelectedSubaccountCount
		selectResourceKinds = originalSelectResourceKinds
		resourceExportFn = originalResourceExportFn
	})

	var operations []string
	getSelectedSubaccountCount = func(context.Context, *btpcli.BtpCli) (int, error) {
		operations = append(operations, "subaccount selection")
		return 1, nil
	}
	selectResourceKinds = func(context.Context) ([]string, error) {
		operations = append(operations, "kind selection")
		return []string{"test-kind"}, nil
	}
	resourceExportFn = func(kind string) func(context.Context, *btpcli.BtpCli, export.EventHandler, resources.Options) error {
		require.Equal(t, "test-kind", kind)
		return func(_ context.Context, _ *btpcli.BtpCli, _ export.EventHandler, options resources.Options) error {
			require.False(t, options.SelectAll)
			operations = append(operations, "child resource selection")
			return nil
		}
	}

	require.NoError(t, exportCmd(context.Background(), noopEventHandler{}))
	require.Equal(t, []string{"subaccount selection", "kind selection", "child resource selection"}, operations)
}

func TestExportCmdSelectiveNonInteractiveExportUsesConfiguredKinds(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	originalGetSelectedSubaccountCount := getSelectedSubaccountCount
	originalSelectResourceKinds := selectResourceKinds
	originalResourceExportFn := resourceExportFn
	t.Cleanup(func() {
		getSelectedSubaccountCount = originalGetSelectedSubaccountCount
		selectResourceKinds = originalSelectResourceKinds
		resourceExportFn = originalResourceExportFn
	})

	getSelectedSubaccountCount = func(context.Context, *btpcli.BtpCli) (int, error) {
		return 1, nil
	}
	selectResourceKinds = export.ResourceKindParam.ValueOrAsk
	viper.Set(export.ResourceKindParam.GetName(), []string{"test-kind"})

	var exportedKinds []string
	resourceExportFn = func(kind string) func(context.Context, *btpcli.BtpCli, export.EventHandler, resources.Options) error {
		return func(_ context.Context, _ *btpcli.BtpCli, _ export.EventHandler, options resources.Options) error {
			require.False(t, options.SelectAll)
			exportedKinds = append(exportedKinds, kind)
			return nil
		}
	}

	require.NoError(t, exportCmd(context.Background(), noopEventHandler{}))
	require.Equal(t, []string{"test-kind"}, exportedKinds)
}

func TestValidateExportAllSubaccountSelection(t *testing.T) {
	tests := []struct {
		name      string
		selectAll bool
		count     int
		wantErr   string
	}{
		{name: "allows one matching subaccount", selectAll: true, count: 1},
		{name: "rejects no matching subaccounts", selectAll: true, count: 0, wantErr: "match exactly one subaccount; matched 0"},
		{name: "rejects multiple matching subaccounts", selectAll: true, count: 2, wantErr: "match exactly one subaccount; matched 2"},
		{name: "allows multiple matches for a selective export", selectAll: false, count: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExportAllSubaccountSelection(tt.selectAll, tt.count)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateExportAllOptions(t *testing.T) {
	t.Run("allows independent and subaccount options", func(t *testing.T) {
		params := testConfigParams(map[string]bool{
			"subaccount":                   true,
			paramResolveRefences.GetName(): true,
			"entitlement-auto-assigned":    true,
		})

		require.NoError(t, validateExportAllOptions(true, params))
	})

	t.Run("allows selective resource selectors when all is disabled", func(t *testing.T) {
		params := testConfigParams(map[string]bool{
			export.ResourceKindParam.GetName(): true,
			"serviceinstance":                  true,
		})

		require.NoError(t, validateExportAllOptions(false, params))
	})

	t.Run("rejects kind selector", func(t *testing.T) {
		params := testConfigParams(map[string]bool{
			export.ResourceKindParam.GetName(): true,
		})

		require.ErrorContains(t, validateExportAllOptions(true, params), "--kind")
	})

	for _, kind := range resources.KindNames() {
		if kind == "subaccount" {
			continue
		}

		t.Run("rejects "+kind+" selector", func(t *testing.T) {
			params := testConfigParams(map[string]bool{kind: true})

			require.ErrorContains(t, validateExportAllOptions(true, params), "--"+kind)
		})
	}
}

func TestExportAllConfigurationAndFlagAreEquivalent(t *testing.T) {
	configSelection := exportAllSelectionFromConfig(t)
	flagSelection := exportAllSelectionFromFlag(t)

	require.Equal(t, configSelection, flagSelection)
	require.True(t, configSelection.SelectAll)
	require.Equal(t, []string{"first", "second"}, selectTestResources(t, configSelection))
	require.Equal(t, []string{"first", "second"}, selectTestResources(t, flagSelection))
}

func exportAllSelectionFromConfig(t *testing.T) resources.Options {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)

	command := &cobra.Command{}
	paramAll.AttachToCommand(command)
	paramAll.BindConfiguration(command)
	viper.SetConfigType("yaml")
	require.NoError(t, viper.ReadConfig(strings.NewReader("all: true\n")))

	return resources.Options{SelectAll: paramAll.Value()}
}

func exportAllSelectionFromFlag(t *testing.T) resources.Options {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)

	command := &cobra.Command{}
	paramAll.AttachToCommand(command)
	paramAll.BindConfiguration(command)
	require.NoError(t, command.PersistentFlags().Set("all", "true"))

	return resources.Options{SelectAll: paramAll.Value()}
}

func selectTestResources(t *testing.T, options resources.Options) []string {
	t.Helper()

	cache := resources.NewResourceCache[*testResource]()
	cache.Store(
		&testResource{id: "first", displayName: "First"},
		&testResource{id: "second", displayName: "Second"},
	)
	param := configparam.StringSlice("resource", "resource selector").
		WithPossibleValuesFn(func() ([]string, error) {
			return nil, errors.New("interactive selector must not be invoked")
		})

	require.NoError(t, resources.SelectCache(context.Background(), cache, param, options))
	return cache.AllIDs()
}

type noopEventHandler struct{}

func (noopEventHandler) Warn(error) {}

func (noopEventHandler) Resource(xpresource.Object) {}

func (noopEventHandler) Stop() {}

type testResource struct {
	id          string
	displayName string
}

func (r *testResource) GetID() string {
	return r.id
}

func (r *testResource) GetDisplayName() string {
	return r.displayName
}

func (*testResource) GetExternalName() string {
	return ""
}

func (*testResource) GenerateK8sResourceName() string {
	return ""
}

func testConfigParams(setParams map[string]bool) configparam.ParamList {
	params := make(configparam.ParamList, 0, len(setParams))
	for name, isSet := range setParams {
		params = append(params, testConfigParam{name: name, set: isSet})
	}
	return params
}

type testConfigParam struct {
	name string
	set  bool
}

func (p testConfigParam) GetName() string {
	return p.name
}

func (p testConfigParam) IsSet() bool {
	return p.set
}

func (testConfigParam) AttachToCommand(*cobra.Command) {}

func (testConfigParam) BindConfiguration(*cobra.Command) {}
