package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"

	// go get github.com/SAP/crossplane-provider-cloudfoundry@5aaac3d6e693b6250e9d492bd0cdd8217cb2ef45
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/configparam"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/export"
	_ "github.com/sap/crossplane-provider-btp/internal/exporttool/cli/export"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/erratt"
)

const (
	shortName      = "btp"
	observedSystem = "SAP Business Technology Platform"

	envVarCISSecret  = "CIS_CENTRAL_BINDING"
	envVarUserSecret = "BTP_TECHNICAL_USER"

	flagNameCISSecret  = "cred-cis"
	flagNameUserSecret = "cred-user"
)

var (
	cisSecretParam = configparam.String(flagNameCISSecret, "If omitted, the value of "+envVarCISSecret+" environment variable is used.\nSee https://github.com/SAP/crossplane-provider-btp for more details.").
		WithFlagName(flagNameCISSecret).
		WithEnvVarName(envVarCISSecret)
	userSecretParam = configparam.SensitiveString(flagNameUserSecret, "If omitted, be the value of the "+envVarUserSecret+" environment variable is used.\nSee https://github.com/SAP/crossplane-provider-btp for more details.").
		WithFlagName(flagNameUserSecret).
		WithEnvVarName(envVarUserSecret).
		WithExample("{\"username\": \"P-User\",\"password\":\"p_user_password\",\"email\":\"p.user@email.address\"}")
	resolveRefencesParam = configparam.Bool("resolve-references", "Resolve inter-resource references").
		WithShortName("r").
		WithEnvVarName("RESOLVE_REFERENCES")
)

func main() {
	cli.Configuration.ShortName = shortName
	cli.Configuration.ObservedSystem = observedSystem
	export.SetCommand(exportCmd)
	export.AddConfigParams(
		cisSecretParam,
		userSecretParam,
		resolveRefencesParam,
	)
	export.AddConfigParams(resources.ConfigParams()...)
	export.AddResourceKinds("subaccount")
	cli.Execute()
}

func exportCmd(ctx context.Context, eventHandler export.EventHandler) error {
	defer eventHandler.Stop()

	// Determine, which kinds the user would like to have exported.
	// selectedResources := []string{"subaccount"} // Uncomment for debugging in the IDE.
	selectedResources, err := export.ResourceKindParam.ValueOrAsk(ctx) // Comment for debugging in the IDE.
	if err != nil {
		return erratt.Errorf("cannot get the value for resource kind parameter: %w", err)
	}
	slog.Debug("kinds selected", "kinds", selectedResources)

	// Connect to BTP API
	cisSecret := []byte(cisSecretParam.Value())
	userSecret := []byte(userSecretParam.Value())

	btpClient, err := btp.ServiceClientFromSecret(cisSecret, userSecret)
	if err != nil {
		return fmt.Errorf("failed to create BTP Client: %w", err)
	}
	slog.DebugContext(ctx, "Successfully acquired BTP accounts service client.")

	// Export selected kinds.
	for _, kind := range selectedResources {
		if eFn := resources.ExportFn(kind); eFn != nil {
			if err := eFn(ctx, &btpClient, eventHandler, resolveRefencesParam.Value()); err != nil {
				eventHandler.Warn(erratt.Errorf("failed to call export function for kind: %w", err).With("kind", kind))
			}
		} else {
			eventHandler.Warn(erratt.New("unknown resource kind", "kind", kind))
		}
	}

	return nil
}
