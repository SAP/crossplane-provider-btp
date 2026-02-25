package main

import (
	"context"
	"log/slog"

	"github.com/SAP/xp-clifford/cli"
	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/erratt"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/cfenvironment"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/entitlement"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceinstance"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
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
	paramBtpCliPath = configparam.String(flagNameBtpCliPath, "Path to the BTP CLI binary that should be used by the export tool to access BTP. Default: 'btp' in your $PATH.").
		WithFlagName(flagNameBtpCliPath).
		WithEnvVarName(envVarBtpCliPath)
)

func main() {
	cli.Configuration.ShortName = shortName
	cli.Configuration.ObservedSystem = observedSystem
	export.SetCommand(exportCmd)
	export.AddConfigParams(
		paramResolveRefences,
		paramBtpCliPath,
	)
	export.AddConfigParams(resources.ConfigParams()...)
	export.AddResourceKinds(resources.KindNames()...)
	cli.Execute()
}

func exportCmd(ctx context.Context, eventHandler export.EventHandler) error {
	defer eventHandler.Stop()

	// Determine, which kinds the user would like to have exported.
	selectedResources, err := export.ResourceKindParam.ValueOrAsk(ctx)
	if err != nil {
		return erratt.Errorf("cannot get the value for resource kind parameter: %w", err)
	}
	slog.Debug("Kinds selected", "kinds", selectedResources)

	// This client does not try to log in, thus relying on existing session.
	// Explicit authentication can be done by a separate `login` command or by BTP CLI's `login` command.
	btpClient := btpcli.NewClient(paramBtpCliPath.Value())

	// Export selected kinds.
	for _, kind := range selectedResources {
		if eFn := resources.ExportFn(kind); eFn != nil {
			if err := eFn(ctx, btpClient, eventHandler, paramResolveRefences.Value()); err != nil {
				eventHandler.Warn(erratt.Errorf("failed to call export function for kind: %w", err).With("kind", kind))
			}
		} else {
			eventHandler.Warn(erratt.New("unknown resource kind", "kind", kind))
		}
	}

	return nil
}
