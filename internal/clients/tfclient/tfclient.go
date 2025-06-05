package tfclient

import (
	"context"
	"encoding/json"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	tjcontroller "github.com/crossplane/upjet/pkg/controller"
	"github.com/crossplane/upjet/pkg/controller/handler"
	"github.com/crossplane/upjet/pkg/terraform"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/config"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	errNoProviderConfig            = "no providerConfigRef provided"
	errGetProviderConfig           = "cannot get referenced ProviderConfig"
	errTrackUsage                  = "cannot track ProviderConfig usage"
	errExtractCredentials          = "cannot extract credentials"
	errUnmarshalCredentials        = "cannot unmarshal btp-account-tf credentials as JSON"
	errTrackRUsage                 = "cannot track ResourceUsage"
	errGetServiceAccountCreds      = "cannot get Service Account credentials"
	errCouldNotParseUserCredential = "error while parsing sa-provider-secret JSON"
)

var (
	// TF_VERSION_CALLBACK is a function callback to allow retrieval of Terraform env versions, its suppose to be set in
	// the main method to the params being passed when starting the controller
	// unfortunately, the way controllers are generically being initialized there is no other way to pass that downstream properly
	TF_VERSION_CALLBACK = func() TfEnvVersion {
		return TfEnvVersion{
			// should reset from within main, these are just tested defaults
			Version:         "1.3.9",
			Providerversion: "1.0.0-rc1",
			ProviderSource:  "SAP/btp",
		}
	}
)

// TerraformSetupBuilder builds Terraform a terraform.SetupFn function which
// returns Terraform provider setup configuration
func TerraformSetupBuilder(version, providerSource, providerVersion string) terraform.SetupFn {
	return func(ctx context.Context, client client.Client, mg resource.Managed) (terraform.Setup, error) {
		ps := terraform.Setup{
			Version: version,
			Requirement: terraform.ProviderRequirement{
				Source:  providerSource,
				Version: providerVersion,
			},
		}

		configRef := mg.GetProviderConfigReference()
		if configRef == nil {
			return ps, errors.New(errNoProviderConfig)
		}

		pc, err := providerconfig.ResolveProviderConfig(ctx, mg, client)
		if err != nil {
			return ps, errors.Wrap(err, errGetProviderConfig)
		}

		t := resource.NewProviderConfigUsageTracker(client, &v1alpha1.ProviderConfigUsage{})
		if err := t.Track(ctx, mg); err != nil {
			return ps, errors.Wrap(err, errTrackUsage)
		}

		if err = tracking.NewDefaultReferenceResolverTracker(client).Track(ctx, mg); err != nil {
			return ps, errors.Wrap(err, errTrackRUsage)
		}

		cd := pc.Spec.ServiceAccountSecret
		ServiceAccountSecretData, err := resource.CommonCredentialExtractor(
			ctx,
			cd.Source,
			client,
			cd.CommonCredentialSelectors,
		)
		if err != nil {
			return ps, errors.Wrap(err, errGetServiceAccountCreds)
		}
		if ServiceAccountSecretData == nil {
			return ps, errors.New(errGetServiceAccountCreds)
		}

		var userCredential btp.UserCredential
		if err := json.Unmarshal(ServiceAccountSecretData, &userCredential); err != nil {
			return ps, errors.Wrap(err, errCouldNotParseUserCredential)
		}

		ps.Configuration = map[string]any{
			"username":       userCredential.Username,
			"password":       userCredential.Password,
			"globalaccount":  pc.Spec.GlobalAccount,
			"cli_server_url": pc.Spec.CliServerUrl,
		}
		return ps, nil
	}
}

func TerraformSetupBuilderNoTracking(version, providerSource, providerVersion string) terraform.SetupFn {
	return func(ctx context.Context, client client.Client, mg resource.Managed) (terraform.Setup, error) {
		ps := terraform.Setup{
			Version: version,
			Requirement: terraform.ProviderRequirement{
				Source:  providerSource,
				Version: providerVersion,
			},
		}

		pc, err := providerconfig.ResolveProviderConfig(ctx, mg, client)
		if err != nil {
			return ps, errors.Wrap(err, errGetProviderConfig)
		}

		cd := pc.Spec.ServiceAccountSecret
		ServiceAccountSecretData, err := resource.CommonCredentialExtractor(
			ctx,
			cd.Source,
			client,
			cd.CommonCredentialSelectors,
		)
		if err != nil {
			return ps, errors.Wrap(err, errGetServiceAccountCreds)
		}
		if ServiceAccountSecretData == nil {
			return ps, errors.New(errGetServiceAccountCreds)
		}

		var userCredential btp.UserCredential
		if err := json.Unmarshal(ServiceAccountSecretData, &userCredential); err != nil {
			return ps, errors.Wrap(err, errCouldNotParseUserCredential)
		}

		ps.Configuration = map[string]any{
			"username":       userCredential.Username,
			"password":       userCredential.Password,
			"globalaccount":  pc.Spec.GlobalAccount,
			"cli_server_url": pc.Spec.CliServerUrl,
		}
		return ps, nil
	}
}

// NewInternalTfConnector creates a new internal Terraform connector, it does not have a callback handler, since those won't be managed by the controller manager
func NewInternalTfConnector(client client.Client, resourceName string, gvk schema.GroupVersionKind) *tjcontroller.Connector {
	tfVersion := TF_VERSION_CALLBACK()
	zl := zap.New(zap.UseDevMode(tfVersion.DebugLogs))
	setupFn := TerraformSetupBuilderNoTracking(tfVersion.Version, tfVersion.ProviderSource, tfVersion.Providerversion)
	log := logging.NewLogrLogger(zl.WithName("crossplane-provider-btp"))
	ws := terraform.NewWorkspaceStore(log)
	provider := config.GetProvider()
	eventHandler := handler.NewEventHandler(handler.WithLogger(log.WithValues("gvk", gvk)))

	connector := tjcontroller.NewConnector(client, ws, setupFn,
		provider.Resources[resourceName],
		tjcontroller.WithLogger(log),
		tjcontroller.WithConnectorEventHandler(eventHandler),
	)

	return connector
}

type TfEnvVersion struct {
	Version         string
	Providerversion string
	ProviderSource  string

	DebugLogs bool
}
