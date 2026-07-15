package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/SAP/xp-clifford/cli"
	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/SAP/xp-clifford/erratt"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
)

const (
	envVarBtpCliServer     = "BTP_EXPORT_BTP_CLI_SERVER_URL"
	envVarUserName         = "BTP_EXPORT_USER_NAME"
	envVarPassword         = "BTP_EXPORT_PASSWORD"
	envVarGlobalAccount    = "BTP_EXPORT_GLOBAL_ACCOUNT"
	envVarIdentityProvider = "BTP_EXPORT_IDP"

	flagNameBtpCliServer     = "url"
	flagNameUser             = "username"
	flagNamePassword         = "password"
	flagNameGlobalAccount    = "subdomain"
	flagNameIdentityProvider = "idp"
	flagNameSSO              = "sso"
)

var (
	paramUserName = configparam.String(flagNameUser, "User name to log in to a global account of SAP BTP, usually an e-mail address.").
		WithFlagName(flagNameUser).
		WithShortName("u").
		WithEnvVarName(envVarUserName)
	paramPassword = configparam.SensitiveString(flagNamePassword, "User password, see recommendations on https://help.sap.com/docs/btp/btp-cli-command-reference/btp-login.").
		WithFlagName(flagNamePassword).
		WithShortName("p").
		WithEnvVarName(envVarPassword)
	paramGlobalAccount = configparam.String(flagNameGlobalAccount, "The subdomain of the global account to export resources from. Can be found, e.g., in BTP Cockpit.").
		WithFlagName(flagNameGlobalAccount).
		WithEnvVarName(envVarGlobalAccount)
	paramBtpCliServer = configparam.String(flagNameBtpCliServer, "The URL of the BTP CLI server. Default: 'https://cli.btp.cloud.sap'").
		WithFlagName(flagNameBtpCliServer).
		WithEnvVarName(envVarBtpCliServer)
	paramIdp = configparam.String(flagNameIdentityProvider, "Origin of the custom identity provider, if configured for the global account.").
		WithFlagName(flagNameIdentityProvider).
		WithEnvVarName(envVarIdentityProvider)
	paramSSO = configparam.Bool(flagNameSSO, "Opens a browser for single sign-on.").
		WithFlagName(flagNameSSO)

	loginSubCommand = &cli.BasicSubCommand{
		Name:             "login",
		Short:            fmt.Sprintf("Log in to a global account of %s", observedSystem),
		Long:             fmt.Sprintf("Log in to a global account of %s", observedSystem),
		IgnoreConfigFile: true,
		ConfigParams: configparam.ParamList{
			paramBtpCliPath,
			paramUserName,
			paramPassword,
			paramGlobalAccount,
			paramBtpCliServer,
			paramIdp,
			paramSSO,
		},
	}
)

func init() {
	loginSubCommand.Run = login
	cli.RegisterSubCommand(loginSubCommand)
}

func login(ctx context.Context) error {
	btpClient := btpcli.NewClient(paramBtpCliPath.Value())

	// If login with SSO is requested, do it without asking further questions.
	if paramSSO.Value() {
		return btpClient.Login(ctx, &btpcli.LoginParameters{
			UserName:               paramUserName.Value(),
			Password:               paramPassword.Value(),
			GlobalAccountSubdomain: paramGlobalAccount.Value(),
			ServerURL:              paramBtpCliServer.Value(),
			IdentityProvider:       paramIdp.Value(),
			SSO:                    paramSSO.Value(),
		})
	}

	username, err := paramUserName.ValueOrAsk(ctx)
	if err != nil {
		return erratt.New("Cannot get user name parameter")
	}
	password, err := paramPassword.ValueOrAsk(ctx)
	if err != nil {
		return erratt.New("Cannot get password parameter")
	}
	subdomain, err := paramGlobalAccount.ValueOrAsk(ctx)
	if err != nil {
		return erratt.New("Cannot get global account subdomain parameter")
	}

	err = btpClient.Login(ctx, &btpcli.LoginParameters{
		UserName:               username,
		Password:               password,
		GlobalAccountSubdomain: subdomain,
		ServerURL:              paramBtpCliServer.Value(),
		IdentityProvider:       paramIdp.Value(),
		SSO:                    paramSSO.Value(),
	})
	if err == nil {
		slog.InfoContext(ctx, "Login successful")
	}

	return err
}
