package tfclient

import (
	"context"
	"encoding/json"
	"sync"

	tfprovider "github.com/SAP/terraform-provider-btp/btp/provider"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	tjcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	"github.com/crossplane/upjet/v2/pkg/controller/handler"
	"github.com/crossplane/upjet/v2/pkg/terraform"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
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

// frameworkProvider returns the BTP provider's plugin-framework implementation,
// called in-process by upjet's no-fork client. terraform.Setup.FrameworkProvider
// must be non-nil for framework-reconciled resources; upjet otherwise fails with
// "cannot retrieve framework provider".
//
// Lazy on purpose: btp.SetDebug() runs in main(), so an init-time btp.IsDebug()
// would always read false and debug HTTP tracing would never reach the provider.
var frameworkProvider = sync.OnceValue(func() fwprovider.Provider {
	if btp.IsDebug() {
		return tfprovider.NewWithClient(btp.DebugPrintHTTPClient())
	}
	return tfprovider.New()
})

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
			FrameworkProvider: frameworkProvider(),
		}

		lm, ok := mg.(providerconfig.LegacyManaged)
		if !ok {
			return ps, errors.New(errNoProviderConfig)
		}
		configRef := lm.GetProviderConfigReference()
		if configRef == nil {
			return ps, errors.New(errNoProviderConfig)
		}

		pc, err := providerconfig.ResolveProviderConfig(ctx, lm, client)
		if err != nil {
			return ps, errors.Wrap(err, errGetProviderConfig)
		}

		t := resource.NewLegacyProviderConfigUsageTracker(client, &v1alpha1.ProviderConfigUsage{})
		if err := t.Track(ctx, lm); err != nil {
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

		// Set custom idp if provided
		if userCredential.Idp != "" {
			ps.Configuration["idp"] = userCredential.Idp
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
			FrameworkProvider: frameworkProvider(),
		}

		lm, ok := mg.(providerconfig.LegacyManaged)
		if !ok {
			return ps, errors.New(errNoProviderConfig)
		}
		pc, err := providerconfig.ResolveProviderConfig(ctx, lm, client)
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

		// Set custom idp if provided
		if userCredential.Idp != "" {
			ps.Configuration["idp"] = userCredential.Idp
		}

		return ps, nil
	}
}

// NewInternalTfConnector creates the internal Terraform connector for
// resourceName. callbackProvider may be nil where async completion is not
// routed back to a CR.
//
// The client kind comes from the resource's own upjet configuration, not from a
// parameter: a non-nil TerraformPluginFrameworkResource means the resource is
// framework-reconciled (no-fork), so no call site can disagree with
// config/external_name.go.
func NewInternalTfConnector(client client.Client, resourceName string, gvk schema.GroupVersionKind, useAsync bool, callbackProvider tjcontroller.CallbackProvider) managed.ExternalConnector {
	tfVersion := TF_VERSION_CALLBACK()
	zl := zap.New(zap.UseDevMode(tfVersion.DebugLogs))
	setupFn := TerraformSetupBuilderNoTracking(tfVersion.Version, tfVersion.ProviderSource, tfVersion.Providerversion)
	log := logging.NewLogrLogger(zl.WithName("crossplane-provider-btp"))
	provider := config.GetProvider()
	eventHandler := handler.NewEventHandler(handler.WithLogger(log.WithValues("gvk", gvk)))

	res := provider.Resources[resourceName]

	if res.TerraformPluginFrameworkResource != nil {
		// No-fork: the provider's Go functions are called in-process. No workspace
		// on disk, no terraform binary, and no identity injection — the framework
		// client threads resource identity itself via the operation tracker.
		if useAsync {
			return tjcontroller.NewTerraformPluginFrameworkAsyncConnector(client, tjcontroller.NewOperationStore(log), setupFn, res,
				tjcontroller.WithTerraformPluginFrameworkAsyncLogger(log),
				tjcontroller.WithTerraformPluginFrameworkAsyncConnectorEventHandler(eventHandler),
				tjcontroller.WithTerraformPluginFrameworkAsyncCallbackProvider(callbackProvider),
			)
		}
		return tjcontroller.NewTerraformPluginFrameworkConnector(client, setupFn, res, tjcontroller.NewOperationStore(log),
			tjcontroller.WithTerraformPluginFrameworkLogger(log),
		)
	}

	// Fork/CLI path, still used by btp_subaccount_service_binding until issue #692.
	// Identity-injecting Store wraps upjet's WorkspaceStore so that every
	// Workspace() call patches an `identity` block into the on-disk
	// terraform.tfstate, satisfying plugin-framework's post-Read identity
	// check (issue #521). The earlier afero.Fs middleware approach was
	// inert — upjet's WithFs doesn't propagate to FileProducer, see
	// identity_injector.go header.
	ws := terraform.NewWorkspaceStore(log)
	store := NewIdentityInjectingStore(ws, log)

	// depending on the context we might need resources that are async or not.
	// GetProvider() builds a fresh provider per call, so this mutates a private
	// copy; do not memoise it without removing this write first.
	res.UseAsync = useAsync

	return tjcontroller.NewConnector(client, store, setupFn,
		res,
		tjcontroller.WithLogger(log),
		tjcontroller.WithConnectorEventHandler(eventHandler),
		tjcontroller.WithCallbackProvider(callbackProvider),
	)
}

// NewInternalTfConnectorNoFork is the no-fork counterpart of NewInternalTfConnector:
// it calls the TF provider in-process, keeping state in the OperationTrackerStore
// (rebuilt from status.atProvider at Connect). Sync-only, so no callbacks and no
// res.UseAsync write.
func NewInternalTfConnectorNoFork(client client.Client, resourceName string) *tjcontroller.TerraformPluginFrameworkConnector {
	tfVersion := TF_VERSION_CALLBACK()
	zl := zap.New(zap.UseDevMode(tfVersion.DebugLogs))
	setupFn := TerraformSetupBuilderNoTracking(tfVersion.Version, tfVersion.ProviderSource, tfVersion.Providerversion)
	log := logging.NewLogrLogger(zl.WithName("crossplane-provider-btp"))
	res := config.GetProvider().Resources[resourceName]
	ots := tjcontroller.NewOperationStore(log)

	return tjcontroller.NewTerraformPluginFrameworkConnector(
		client, setupFn, res, ots,
		tjcontroller.WithTerraformPluginFrameworkLogger(log),
	)
}

type TfEnvVersion struct {
	Version         string
	Providerversion string
	ProviderSource  string

	DebugLogs bool
}
