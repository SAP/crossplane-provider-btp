package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alecthomas/kingpin/v2"
	tjcontroller "github.com/crossplane/upjet/pkg/controller"
	"github.com/crossplane/upjet/pkg/terraform"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/config"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"github.com/sap/crossplane-provider-btp/internal/features"
	"github.com/sap/crossplane-provider-btp/internal/version"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/feature"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"

	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
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
			"How often all watched resources are re-listed from the API server to correct missed events. "+
				"Technically: controller-runtime cache SyncPeriod; triggers a full re-list per resource type, with 10% jitter between controllers.",
		).Short('s').Default("1h").Duration()
		pollInterval = app.Flag(
			"poll",
			"How often a successfully-reconciled resource is re-checked for drift from the desired state. "+
				"Technically: requeue delay after any successful reconcile (both when up-to-date and after successful updates), since no watch events exist for external resources.",
		).Default("1m").Duration()
		pollJitter = app.Flag(
			"poll-jitter",
			"Random spread added to poll interval to avoid all resources polling at the same instant. "+
				"Technically: random duration between -jitter and +jitter added to poll interval on each requeue. 0 disables jitter.",
		).Default("0").Duration()
		maxReconcileRate = app.Flag(
			"max-reconcile-rate",
			"How many reconciles per second are allowed globally across all controllers. "+
				"Technically: token-bucket rate limiter shared by all controllers; refills at this rate with a burst of 10x.",
		).Default("3").Int()
		maxConcurrentReconciles = app.Flag(
			"max-concurrent-reconciles",
			"How many reconciles can run in parallel per controller. "+
				"Technically: number of worker goroutines dequeuing from the controller-runtime work queue per controller.",
		).Default("3").Int()
		kubeClientQPS = app.Flag(
			"kube-client-qps",
			"How many requests per second the provider may send to the Kubernetes API. "+
				"Recommended: 5x the max-reconcile-rate, since one reconcile typically makes ~5 API calls. "+
				"Technically: token refill rate for the client-go REST client rate limiter (applies to all API calls from this provider).",
		).Default("15").Int()
		kubeClientBurst = app.Flag(
			"kube-client-burst",
			"Maximum burst of requests allowed to the Kubernetes API above the steady QPS. "+
				"Recommended: 2x kube-client-qps. Defaults to 2x kube-client-qps if not explicitly set. "+
				"Technically: bucket size for the client-go REST client rate limiter.",
		).Int()
		reconcileTimeout = app.Flag(
			"reconcile-timeout",
			"Maximum time a single reconcile may take before being canceled. "+
				"Technically: deadline for the externalCtx passed to external API calls; a separate parent context with +30s grace period remains active for K8s status updates after expiry.",
		).Default("3m").Duration()
		backoffBase = app.Flag(
			"backoff-base",
			"Initial wait time before retrying a failed reconcile. "+
				"Technically: base duration for per-item exponential backoff (baseDelay * 2^numFailures) in the controller-runtime work queue.",
		).Default("1s").Duration()
		backoffMax = app.Flag(
			"backoff-max",
			"Maximum wait time between retries of a failing reconcile. "+
				"Technically: cap for the exponential backoff rate limiter.",
		).Default("60s").Duration()
		leaderElectionLeaseDuration = app.Flag(
			"leader-election-lease-duration",
			"How long standby replicas wait before taking over leadership. "+
				"Technically: Kubernetes Lease duration; other candidates wait this long before force-acquiring.",
		).Default("60s").Duration()
		leaderElectionRenewDeadline = app.Flag(
			"leader-election-renew-deadline",
			"How long the active leader tries to renew before giving up. "+
				"Technically: deadline for the leader to refresh the Lease; on expiry the leader steps down and cancels in-flight reconciles.",
		).Default("50s").Duration()
		leaderElectionRetryPeriod = app.Flag(
			"leader-election-retry-period",
			"How often leader election actions (renew or acquire) are attempted. "+
				"Technically: interval between LeaderElector client tries; applies to both the leader refreshing its lease and standbys attempting to acquire.",
		).Default("10s").Duration()

		namespace = app.Flag(
			"namespace",
			"Namespace used to set as default scope in default secret store config.",
		).Default("crossplane-system").Envar("POD_NAMESPACE").String()
		enableExternalSecretStores = app.Flag(
			"enable-external-secret-stores",
			"Enable support for ExternalSecretStores.",
		).Default("false").Envar("ENABLE_EXTERNAL_SECRET_STORES").Bool()
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

	// Default burst to 2x QPS if not explicitly set
	if *kubeClientBurst == 0 {
		*kubeClientBurst = *kubeClientQPS * 2
	}

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

	cfg.QPS = float32(*kubeClientQPS)
	cfg.Burst = *kubeClientBurst

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
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
		LeaseDuration:              leaderElectionLeaseDuration,
		RenewDeadline:              leaderElectionRenewDeadline,
		RetryPeriod:                leaderElectionRetryPeriod,
	})
	kingpin.FatalIfError(err, "Cannot create controller manager")
	kingpin.FatalIfError(apis.AddToScheme(mgr.GetScheme()), "Cannot add Template APIs to scheme")

	setupTerraformControllers(mgr, log, maxReconcileRate, *pollInterval, *pollJitter, *reconcileTimeout, backoffBase, backoffMax, enableManagementPolicies, enableExternalSecretStores, namespace, terraformVersion, providerSource, providerVersion)
	setupNativeControllers(mgr, log, maxReconcileRate, maxConcurrentReconciles, pollInterval, pollJitter, reconcileTimeout, backoffBase, backoffMax, enableManagementPolicies, enableExternalSecretStores, namespace)

	kingpin.FatalIfError(mgr.Start(ctrl.SetupSignalHandler()), "Cannot start controller manager")
}

func setupTerraformControllers(mgr manager.Manager, log logging.Logger, maxReconcileRate *int, pollInterval time.Duration, pollJitter time.Duration, reconcileTimeout time.Duration, backoffBase *time.Duration, backoffMax *time.Duration, enableManagementPolicies *bool, enableExternalSecretStores *bool, namespace *string, terraformVersion *string, providerSource *string, providerVersion *string) {
	o := internalopts.UpjetOptions{
		Options: tjcontroller.Options{
			Options: controller.Options{
				Logger:                  log,
				GlobalRateLimiter:       ratelimiter.NewGlobal(*maxReconcileRate),
				PollInterval:            pollInterval,
				MaxConcurrentReconciles: 1,
				Features:                &feature.Flags{},
			},
			Provider:   config.GetProvider(),
			PollJitter: pollJitter,
			// use the following WorkspaceStoreOption to enable the shared gRPC mode
			// terraform.WithProviderRunner(terraform.NewSharedProvider(log, os.Getenv("TERRAFORM_NATIVE_PROVIDER_PATH"), terraform.WithNativeProviderArgs("-debuggable")))
			WorkspaceStore: terraform.NewWorkspaceStore(log),
			SetupFn:        tfclient.TerraformSetupBuilder(*terraformVersion, *providerSource, *providerVersion),
		},
		BackoffBase: *backoffBase,
		BackoffMax:  *backoffMax,
		Timeout:     reconcileTimeout,
	}

	if *enableManagementPolicies {
		o.Features.Enable(features.EnableBetaManagementPolicies)
		log.Info("Beta feature enabled", "flag", features.EnableBetaManagementPolicies)
	}

	if *enableExternalSecretStores {
		o.Features.Enable(features.EnableAlphaExternalSecretStores)
		log.Info("Alpha feature enabled", "flag", features.EnableAlphaExternalSecretStores)

		// Ensure default store config exists.
		kingpin.FatalIfError(
			resource.Ignore(
				kerrors.IsAlreadyExists, mgr.GetClient().Create(
					context.Background(), &v1alpha1.StoreConfig{
						ObjectMeta: metav1.ObjectMeta{
							Name: "default",
						},
						Spec: v1alpha1.StoreConfigSpec{
							// NOTE(turkenh): We only set required spec and expect optional
							// ones to properly be initialized with CRD level default values.
							SecretStoreConfig: xpv1.SecretStoreConfig{
								DefaultScope: *namespace,
							},
						},
					},
				),
			), "cannot create default store config",
		)
	}

	kingpin.FatalIfError(template.Setup(mgr, o), "Cannot setup controllers")
}
func setupNativeControllers(mgr manager.Manager, log logging.Logger, maxReconcileRate *int, maxConcurrentReconciles *int, pollInterval *time.Duration, pollJitter *time.Duration, reconcileTimeout *time.Duration, backoffBase *time.Duration, backoffMax *time.Duration, enableManagementPolicies *bool, enableExternalSecretStores *bool, namespace *string) {
	co := internalopts.CrossplaneOptions{
		Options: controller.Options{
			Logger:                  log,
			MaxConcurrentReconciles: *maxConcurrentReconciles,
			PollInterval:            *pollInterval,
			GlobalRateLimiter:       ratelimiter.NewGlobal(*maxReconcileRate),
			Features:                &feature.Flags{},
		},
		BackoffBase: *backoffBase,
		BackoffMax:  *backoffMax,
		PollJitter:  *pollJitter,
		Timeout:     *reconcileTimeout,
	}

	if *enableManagementPolicies {
		co.Features.Enable(features.EnableBetaManagementPolicies)
		log.Info("Beta feature enabled", "flag", features.EnableBetaManagementPolicies)
	}

	if *enableExternalSecretStores {
		co.Features.Enable(features.EnableAlphaExternalSecretStores)
		log.Info("Alpha feature enabled", "flag", features.EnableAlphaExternalSecretStores)

		// Ensure default store config exists.
		kingpin.FatalIfError(
			resource.Ignore(
				kerrors.IsAlreadyExists, mgr.GetClient().Create(
					context.Background(), &v1alpha1.StoreConfig{
						ObjectMeta: metav1.ObjectMeta{
							Name: "default",
						},
						Spec: v1alpha1.StoreConfigSpec{
							// NOTE(turkenh): We only set required spec and expect optional
							// ones to properly be initialized with CRD level default values.
							SecretStoreConfig: xpv1.SecretStoreConfig{
								DefaultScope: *namespace,
							},
						},
					},
				),
			), "cannot create default store config",
		)
	}
	kingpin.FatalIfError(template.CustomSetup(mgr, co), "Cannot setup controllers")
}
