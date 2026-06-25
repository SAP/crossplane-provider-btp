package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alecthomas/kingpin/v2"
	tjcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	"github.com/crossplane/upjet/v2/pkg/terraform"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/config"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"github.com/sap/crossplane-provider-btp/internal/features"
	"github.com/sap/crossplane-provider-btp/internal/version"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"

	"github.com/sap/crossplane-provider-btp/apis"
	template "github.com/sap/crossplane-provider-btp/internal/controller"
)

func main() {
	var (
		app            = kingpin.New(filepath.Base(os.Args[0]), "SAP BTP Account Management support for Crossplane.").DefaultEnvars()
		debug          = app.Flag("debug", "Run with debug logging.").Short('d').Bool()
		leaderElection = app.Flag(
			"leader-election",
			"Use leader election for the controller manager.",
		).Short('l').Default("false").OverrideDefaultFromEnvar("LEADER_ELECTION").Bool()

		syncInterval = app.Flag(
			"sync",
			"How often all resources will be double-checked for drift from the desired state.",
		).Short('s').Default("1h").Duration()
		pollInterval = app.Flag(
			"poll",
			"How often individual resources will be checked for drift from the desired state",
		).Default("1m").Duration()
		maxReconcileRate = app.Flag(
			"max-reconcile-rate",
			"The global maximum rate per second at which resources may checked for drift from the desired state.",
		).Default("3").Int()
		backoffBase = app.Flag(
			"backoff-base",
			"Base duration for exponential backoff for reconciling resources in error cases. Default is 1s",
		).Default("1s").Duration()
		backoffMax = app.Flag(
			"backoff-max",
			"Maximum duration for exponential backoff for reconciling resources in error cases. Default is 60s",
		).Default("60s").Duration()

		enableManagementPolicies = app.Flag("enable-management-policies", "Enable support for Management Policies.").Default("true").Envar("ENABLE_MANAGEMENT_POLICIES").Bool()

		terraformVersion = app.Flag("terraform-version", "Terraform version.").Required().Envar("TERRAFORM_VERSION").String()
		providerSource   = app.Flag("terraform-provider-source", "Terraform provider source.").Required().Envar("TERRAFORM_PROVIDER_SOURCE").String()
		providerVersion  = app.Flag("terraform-provider-version", "Terraform provider version.").Required().Envar("TERRAFORM_PROVIDER_VERSION").String()
	)

	tfclient.TF_VERSION_CALLBACK = func() tfclient.TfEnvVersion {
		return tfclient.TfEnvVersion{
			Version:         *terraformVersion,
			Providerversion: *providerVersion,
			ProviderSource:  *providerSource,
			DebugLogs:       *debug,
		}
	}

	kingpin.MustParse(app.Parse(os.Args[1:]))

	zl := zap.New(zap.UseDevMode(*debug))
	log := logging.NewLogrLogger(zl.WithName("crossplane-provider-btp"))
	ctrl.SetLogger(zl)
	btp.SetLogger(log)
	btp.SetDebug(*debug)

	cfg, err := ctrl.GetConfig()
	kingpin.FatalIfError(err, "Cannot get API server rest config")

	// Set custom user agent for terraform http calls via env variable
	envErr := os.Setenv("BTP_APPEND_USER_AGENT", fmt.Sprintf("crossplane/%s", version.ProviderVersion))
	kingpin.FatalIfError(envErr, "Cannot set environment variable BTP_APPEND_USER_AGENT")

	mgr, err := ctrl.NewManager(
		ratelimiter.LimitRESTConfig(cfg, *maxReconcileRate), ctrl.Options{
			Cache: cache.Options{SyncPeriod: syncInterval},

			// controller-runtime uses both ConfigMaps and Leases for leader
			// election by default. Leases expire after 15 seconds, with a
			// 10 second renewal deadline. We've observed leader loss due to
			// renewal deadlines being exceeded when under high load - i.e.
			// hundreds of reconciles per second and ~200rps to the API
			// server. Switching to Leases only and longer leases appears to
			// alleviate this.
			LeaderElection:             *leaderElection,
			LeaderElectionID:           "crossplane-leader-election-crossplane-provider-btp",
			LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
			LeaseDuration:              func() *time.Duration { d := 60 * time.Second; return &d }(),
			RenewDeadline:              func() *time.Duration { d := 50 * time.Second; return &d }(),
		},
	)
	kingpin.FatalIfError(err, "Cannot create controller manager")
	kingpin.FatalIfError(apis.AddToScheme(mgr.GetScheme()), "Cannot add Template APIs to scheme")

	setupTerraformControllers(mgr, log, maxReconcileRate, *pollInterval, backoffBase, backoffMax, enableManagementPolicies, terraformVersion, providerSource, providerVersion)
	setupNativeControllers(mgr, log, maxReconcileRate, pollInterval, backoffBase, backoffMax, enableManagementPolicies)

	kingpin.FatalIfError(mgr.Start(ctrl.SetupSignalHandler()), "Cannot start controller manager")
}

func setupTerraformControllers(mgr manager.Manager, log logging.Logger, maxReconcileRate *int, pollInterval time.Duration, backoffBase *time.Duration, backoffMax *time.Duration, enableManagementPolicies *bool, terraformVersion *string, providerSource *string, providerVersion *string) {
	o := internalopts.UpjetOptions{
		Options: tjcontroller.Options{
			Options: controller.Options{
				Logger:                  log,
				GlobalRateLimiter:       ratelimiter.NewGlobal(*maxReconcileRate),
				PollInterval:            pollInterval,
				MaxConcurrentReconciles: 1,
				Features:                &feature.Flags{},
			},
			Provider: config.GetProvider(),
			// use the following WorkspaceStoreOption to enable the shared gRPC mode
			// terraform.WithProviderRunner(terraform.NewSharedProvider(log, os.Getenv("TERRAFORM_NATIVE_PROVIDER_PATH"), terraform.WithNativeProviderArgs("-debuggable")))
			WorkspaceStore: terraform.NewWorkspaceStore(log),
			SetupFn:        tfclient.TerraformSetupBuilder(*terraformVersion, *providerSource, *providerVersion),
		},
		BackoffBase: *backoffBase,
		BackoffMax:  *backoffMax,
	}

	if *enableManagementPolicies {
		o.Features.Enable(features.EnableBetaManagementPolicies)
		log.Info("Beta feature enabled", "flag", features.EnableBetaManagementPolicies)
	}

	kingpin.FatalIfError(template.Setup(mgr, o), "Cannot setup controllers")
}
func setupNativeControllers(mgr manager.Manager, log logging.Logger, maxReconcileRate *int, pollInterval *time.Duration, backoffBase *time.Duration, backoffMax *time.Duration, enableManagementPolicies *bool) {
	co := internalopts.CrossplaneOptions{
		Options: controller.Options{
			Logger:                  log,
			MaxConcurrentReconciles: *maxReconcileRate,
			PollInterval:            *pollInterval,
			GlobalRateLimiter:       ratelimiter.NewGlobal(*maxReconcileRate),
			Features:                &feature.Flags{},
		},
		BackoffBase: *backoffBase,
		BackoffMax:  *backoffMax,
	}

	if *enableManagementPolicies {
		co.Features.Enable(features.EnableBetaManagementPolicies)
		log.Info("Beta feature enabled", "flag", features.EnableBetaManagementPolicies)
	}

	kingpin.FatalIfError(template.CustomSetup(mgr, co), "Cannot setup controllers")
}
