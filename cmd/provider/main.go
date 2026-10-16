package main

import (
	"crypto/fips140"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alecthomas/kingpin/v2"
	tjcontroller "github.com/crossplane/upjet/v2/pkg/controller"
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
		reconcileTimeout = app.Flag(
			"reconcile-timeout",
			"Timeout for a single reconcile cycle (externalCtx budget). The reconciler adds a 30s grace period on top for status writes. Default is 1m.",
		).Default("1m").Envar("RECONCILE_TIMEOUT").Duration()

		enableTerraformSessionReuse = app.Flag(
			"enable-terraform-session-reuse",
			"DEPRECATED: use --terraform-session-pool-size. true maps to pool size 1; otherwise ignored.",
		).Default("false").Envar("ENABLE_TERRAFORM_SESSION_REUSE").Bool()

		terraformSessionPoolSize = app.Flag(
			"terraform-session-pool-size",
			"Warmed Terraform BTP sessions kept per global account. 'auto' = max-reconcile-rate + 2. 0 = a fresh client per reconcile; 1 = a single shared session.",
		).Default("auto").Envar("TERRAFORM_SESSION_POOL_SIZE").String()

		terraformSessionTTL = app.Flag(
			"terraform-session-ttl",
			"Lifetime of a warmed Terraform BTP session before it is re-authenticated on next use.",
		).Default("2h").Envar("TERRAFORM_SESSION_TTL").Duration()

		enableManagementPolicies = app.Flag("enable-management-policies", "Enable support for Management Policies.").Default("true").Envar("ENABLE_MANAGEMENT_POLICIES").Bool()
	)

	kingpin.MustParse(app.Parse(os.Args[1:]))

	zl := zap.New(zap.UseDevMode(*debug))
	log := logging.NewLogrLogger(zl.WithName("crossplane-provider-btp"))
	ctrl.SetLogger(zl)

	// Log FIPS 140-3 status at startup so the running pod self-evidences whether
	// the Go Cryptographic Module (CMVP #5247) is active in approved mode.
	// With GOFIPS140=certified at build time this will always be true; the log line
	// provides an auditable artifact. See https://go.dev/doc/security/fips140
	log.Info("FIPS 140-3 status",
		"enabled", fips140.Enabled(),
		"module_version", fips140.Version(),
	)

	btp.SetLogger(log)
	btp.SetDebug(*debug)
	tfclient.SetTerraformSessionPool(
		resolveTerraformSessionPoolSize(*terraformSessionPoolSize, *maxReconcileRate, *enableTerraformSessionReuse, log),
		*terraformSessionTTL,
	)

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

	setupTerraformControllers(mgr, log, maxReconcileRate, *pollInterval, backoffBase, backoffMax, enableManagementPolicies)
	setupNativeControllers(mgr, log, maxReconcileRate, pollInterval, reconcileTimeout, backoffBase, backoffMax, enableManagementPolicies)

	kingpin.FatalIfError(mgr.Start(ctrl.SetupSignalHandler()), "Cannot start controller manager")
}

func setupTerraformControllers(mgr manager.Manager, log logging.Logger, maxReconcileRate *int, pollInterval time.Duration, backoffBase *time.Duration, backoffMax *time.Duration, enableManagementPolicies *bool) {
	o := internalopts.UpjetOptions{
		Options: tjcontroller.Options{
			Options: controller.Options{
				Logger:                  log,
				GlobalRateLimiter:       ratelimiter.NewGlobal(*maxReconcileRate),
				PollInterval:            pollInterval,
				MaxConcurrentReconciles: *maxReconcileRate,
				Features:                &feature.Flags{},
			},
			Provider:              config.GetProvider(),
			OperationTrackerStore: tjcontroller.NewOperationStore(log),
			SetupFn:               tfclient.TerraformSetupBuilder(),
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
func setupNativeControllers(mgr manager.Manager, log logging.Logger, maxReconcileRate *int, pollInterval *time.Duration, reconcileTimeout *time.Duration, backoffBase *time.Duration, backoffMax *time.Duration, enableManagementPolicies *bool) {
	co := internalopts.CrossplaneOptions{
		Options: controller.Options{
			Logger:                  log,
			MaxConcurrentReconciles: *maxReconcileRate,
			PollInterval:            *pollInterval,
			GlobalRateLimiter:       ratelimiter.NewGlobal(*maxReconcileRate),
			Features:                &feature.Flags{},
		},
		BackoffBase:      *backoffBase,
		BackoffMax:       *backoffMax,
		ReconcileTimeout: *reconcileTimeout,
	}

	if *enableManagementPolicies {
		co.Features.Enable(features.EnableBetaManagementPolicies)
		log.Info("Beta feature enabled", "flag", features.EnableBetaManagementPolicies)
	}

	kingpin.FatalIfError(template.CustomSetup(mgr, co), "Cannot setup controllers")
}

// resolveTerraformSessionPoolSize turns the --terraform-session-pool-size flag into
// a concrete size. "auto" = max-reconcile-rate + 2. The deprecated reuse flag is
// honoured only when it opts in (true -> 1); otherwise it is ignored.
func resolveTerraformSessionPoolSize(flag string, maxReconcileRate int, deprecatedReuse bool, log logging.Logger) int {
	if flag != "auto" {
		n, err := strconv.Atoi(flag)
		kingpin.FatalIfError(err, "Invalid --terraform-session-pool-size (want 'auto' or an integer >= 0)")
		if n < 0 {
			kingpin.Fatalf("--terraform-session-pool-size must be >= 0, got %d", n)
		}
		return n
	}
	if deprecatedReuse {
		log.Info("--enable-terraform-session-reuse is deprecated; mapping to --terraform-session-pool-size=1")
		return 1
	}
	return maxReconcileRate + 2
}
