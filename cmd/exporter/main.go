package main

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/SAP/xp-clifford/cli"
	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/erratt"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/cfenvironment"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/directory"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/entitlement"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/scope"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/servicebinding"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceinstance"
)

const (
	shortName      = "btp"
	observedSystem = "SAP BTP"

	envVarBtpCliPath   = "BTP_EXPORT_BTP_CLI_PATH"
	flagNameBtpCliPath = "btp-cli"
)

var (
	paramResolveRefences = configparam.Bool("resolve-references", "Resolve inter-resource references").
				WithShortName("r").
				WithEnvVarName("RESOLVE_REFERENCES")
	paramAll = configparam.Bool("all", "Export all supported resource kinds and matching resources.").
			WithFlagName("all")
	paramBtpCliPath = configparam.String(flagNameBtpCliPath, "Path to the BTP CLI binary that should be used by the export tool to access BTP. Default: 'btp' in your $PATH.").
			WithFlagName(flagNameBtpCliPath).
			WithEnvVarName(envVarBtpCliPath)

	getSelectedRootCount = func(ctx context.Context, btpClient *btpcli.BtpCli) (int, error) {
		return scope.RootCount(ctx, btpClient)
	}
	selectResourceKinds = export.ResourceKindParam.ValueOrAsk
	resourceExportFn    = resources.ExportFn
)

func main() {
	cli.Configuration.ShortName = shortName
	cli.Configuration.ObservedSystem = observedSystem
	export.SetCommand(exportCmd)
	export.AddConfigParams(
		paramResolveRefences,
		paramAll,
		paramBtpCliPath,
	)
	export.AddConfigParams(scope.Param())
	export.AddConfigParams(resources.ConfigParams()...)
	export.AddResourceKinds(resources.KindNames()...)
	cli.Execute()
}

func exportCmd(ctx context.Context, eventHandler export.EventHandler) error {
	defer eventHandler.Stop()

	if err := validateExportAllOptions(paramAll.Value(), export.GetConfigParams()); err != nil {
		return erratt.Errorf("invalid export selection: %w", err)
	}

	// This client does not try to log in, thus relying on an existing session.
	// Explicit authentication can be done by a separate `login` command or by BTP CLI's `login` command.
	btpClient := btpcli.NewClient(paramBtpCliPath.Value())

	// Resolve the combined subaccount/directory scope selection before asking
	// which resource kinds to export. Every exporter reuses this cached selection.
	//
	// Under --all the picker still prompts for the scope when --subaccount was
	// not passed on the command line. The one-root guard below only applies to a
	// non-interactive scope (an explicit --subaccount selector), where a too
	// broad value should fail loud rather than silently export several roots.
	// IsSet() must be read before selection runs: a successful interactive
	// prompt stores its result and would otherwise make IsSet() report true.
	scopeProvidedViaFlag := scope.Param().(interface{ IsSet() bool }).IsSet()

	selectedRootCount, err := getSelectedRootCount(ctx, btpClient)
	if err != nil {
		return erratt.Errorf("cannot retrieve and select export scope: %w", err)
	}
	if scopeProvidedViaFlag {
		if err := validateExportAllSubaccountSelection(paramAll.Value(), selectedRootCount); err != nil {
			return erratt.Errorf("invalid export selection: %w", err)
		}
	}

	// Determine which kinds the user would like to have exported.
	var selectedResources []string
	if paramAll.Value() {
		selectedResources = resources.KindNames()
	} else {
		var err error
		selectedResources, err = selectResourceKinds(ctx)
		if err != nil {
			return erratt.Errorf("cannot get the value for resource kind parameter: %w", err)
		}
	}
	slog.Debug("Kinds selected", "kinds", selectedResources)

	// Export selected kinds.
	options := resources.Options{
		ResolveReferences: paramResolveRefences.Value(),
		SelectAll:         paramAll.Value(),
	}
	for _, kind := range selectedResources {
		if eFn := resourceExportFn(kind); eFn != nil {
			if err := eFn(ctx, btpClient, eventHandler, options); err != nil {
				eventHandler.Warn(erratt.Errorf("failed to call export function for kind: %w", err).With("kind", kind))
			}
		} else {
			eventHandler.Warn(erratt.New("unknown resource kind", "kind", kind))
		}
	}

	return nil
}

func validateExportAllSubaccountSelection(selectAll bool, selectedRootCount int) error {
	if !selectAll || selectedRootCount == 1 {
		return nil
	}

	return fmt.Errorf("--all requires the scope selector to match exactly one subaccount or directory; matched %d", selectedRootCount)
}

func validateExportAllOptions(selectAll bool, configParams configparam.ParamList) error {
	if !selectAll {
		return nil
	}

	var conflicts []string
	for _, param := range configParams {
		flagName, isResourceSelector := exportAllConflictFlagName(param.GetName())
		if !isResourceSelector {
			continue
		}

		presenceAwareParam, ok := param.(interface{ IsSet() bool })
		if !ok {
			return fmt.Errorf("cannot determine whether configuration parameter %q was set", param.GetName())
		}
		if presenceAwareParam.IsSet() {
			conflicts = append(conflicts, flagName)
		}
	}
	if len(conflicts) == 0 {
		return nil
	}

	sort.Strings(conflicts)
	return fmt.Errorf("--all cannot be combined with explicit resource selectors: %s", strings.Join(conflicts, ", "))
}

func exportAllConflictFlagName(paramName string) (string, bool) {
	if paramName == export.ResourceKindParam.GetName() {
		return "--kind", true
	}
	for _, kind := range resources.KindNames() {
		// The subaccount and directory selectors identify the export scope (the
		// source subaccounts/directories) for an export-all operation; they do
		// not filter which kinds are exported. Both are owned by the scope
		// package's combined selector.
		if kind == paramName && kind != "subaccount" && kind != "directory" {
			return "--" + kind, true
		}
	}
	return "", false
}
