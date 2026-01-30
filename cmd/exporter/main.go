package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli"
	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/erratt"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/entitlement"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceinstance"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

const (
	shortName      = "btp"
	observedSystem = "SAP Business Technology Platform"

	envVarBtpCliPath       = "BTP_EXPORT_BTP_CLI_PATH"
	envVarBtpCliServer     = "BTP_EXPORT_BTP_CLI_SERVER_URL"
	envVarUserName         = "BTP_EXPORT_USER_NAME"
	envVarPassword         = "BTP_EXPORT_PASSWORD"
	envVarGlobalAccount    = "BTP_EXPORT_GLOBAL_ACCOUNT"
	envVarIdentityProvider = "BTP_EXPORT_IDP"

	flagNameBtpCliPath       = "btp-cli-path"
	flagNameBtpCliServer     = "url"
	flagNameUser             = "username"
	flagNamePassword         = "password"
	flagNameGlobalAccount    = "subdomain"
	flagNameIdentityProvider = "idp"
)

var (
	paramResolveRefences = configparam.Bool("resolve-references", "Resolve inter-resource references").
		WithShortName("r").
		WithEnvVarName("RESOLVE_REFERENCES")
	paramUserName = configparam.String(flagNameUser, "User name to log in to a global account of SAP BTP.").
		WithFlagName(flagNameUser).
		WithEnvVarName(envVarUserName)
	paramPassword = configparam.String(flagNamePassword, "User password to log in to a global account of SAP BTP.").
		WithFlagName(flagNamePassword).
		WithEnvVarName(envVarPassword)
	paramGlobalAccount = configparam.String(flagNameGlobalAccount, "The subdomain of the global account to export resources from.").
		WithFlagName(flagNameGlobalAccount).
		WithEnvVarName(envVarGlobalAccount)
	paramBtpCliPath = configparam.String(flagNameBtpCliPath, "Path to the BTP CLI binary that should be by the export tool to access BTP. Default: 'btp' in your $PATH.").
		WithFlagName(flagNameBtpCliPath).
		WithEnvVarName(envVarBtpCliPath)
	paramBtpCliServer = configparam.String(flagNameBtpCliServer, "The URL of the BTP CLI server. Default: 'https://cli.btp.cloud.sap'").
		WithFlagName(flagNameBtpCliServer).
		WithEnvVarName(envVarBtpCliServer)
	paramIdp = configparam.String(flagNameIdentityProvider, "Origin of the custom identity provider, if configured for the global account.").
		WithFlagName(flagNameIdentityProvider).
		WithEnvVarName(envVarIdentityProvider)
)

func main() {
	cli.Configuration.ShortName = shortName
	cli.Configuration.ObservedSystem = observedSystem
	export.SetCommand(exportCmd)
	export.AddConfigParams(
		paramResolveRefences,
		paramUserName,
		paramPassword,
		paramGlobalAccount,
		paramBtpCliPath,
		paramBtpCliServer,
		paramIdp,
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

	// Connect to BTP API
	btpClient, err := btpcli.NewClientAndLogin(ctx,
		paramBtpCliPath.Value(),
		&btpcli.LoginParameters{
			UserName:               paramUserName.Value(),
			Password:               paramPassword.Value(),
			GlobalAccountSubdomain: paramGlobalAccount.Value(),
			ServerURL:              paramBtpCliServer.Value(),
			IdentityProvider:       paramIdp.Value(),
		})
	if err != nil {
		return fmt.Errorf("failed to create BTP Client: %w", err)
	}

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
