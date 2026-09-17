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

// TerraformSetupBuilder builds a terraform.SetupFn for the generated
// (standalone) upjet controllers. It resolves the ProviderConfig and tracks
// its usage. The returned Setup only carries FrameworkProvider + Configuration:
// no-fork calls the provider in-process, so Version/Requirement are unused.
func TerraformSetupBuilder() terraform.SetupFn {
	return func(ctx context.Context, client client.Client, mg resource.Managed) (terraform.Setup, error) {
		ps := terraform.Setup{
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

// TerraformSetupBuilderNoTracking is the setup builder for the hybrid
// (class-2) internal connectors. It skips ProviderConfigUsage / reference
// tracking because the outer native controller already tracks the
// user-facing CR.
func TerraformSetupBuilderNoTracking() terraform.SetupFn {
	return func(ctx context.Context, client client.Client, mg resource.Managed) (terraform.Setup, error) {
		ps := terraform.Setup{
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
// routed back to a CR. Every configured resource is framework-reconciled
// (no-fork): the provider's Go functions are called in-process, no workspace on
// disk, no terraform binary, and no identity injection — the framework client
// threads resource identity itself via the operation tracker.
func NewInternalTfConnector(client client.Client, resourceName string, gvk schema.GroupVersionKind, useAsync bool, callbackProvider tjcontroller.CallbackProvider) managed.ExternalConnector {
	zl := zap.New(zap.UseDevMode(btp.IsDebug()))
	setupFn := TerraformSetupBuilderNoTracking()
	log := logging.NewLogrLogger(zl.WithName("crossplane-provider-btp"))
	res := config.GetProvider().Resources[resourceName]

	if useAsync {
		eventHandler := handler.NewEventHandler(handler.WithLogger(log.WithValues("gvk", gvk)))
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
