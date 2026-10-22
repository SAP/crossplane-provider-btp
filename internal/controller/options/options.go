// Package options provides controller options helpers for controllers in this provider.
package options

import (
	"time"

	xpcontroller "github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	tjcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	"github.com/sap/crossplane-provider-btp/internal/controller/pollinterval"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	crcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// RuntimeOptionsGenerator abstracts the generation of controller-runtime options from native xp options and upjet options
type RuntimeOptionsGenerator interface {
	ForControllerRuntime() crcontroller.Options
	ForControllerRuntimeWithBackoff() crcontroller.Options
}

var _ RuntimeOptionsGenerator = CrossplaneOptions{}

// CrossplaneOptions is a wrapper for adding more configuration on top of default xp controller options
type CrossplaneOptions struct {
	xpcontroller.Options

	BackoffBase          time.Duration
	BackoffMax           time.Duration
	ReconcileTimeout     time.Duration
	PollJitterPercentage uint
	PollMax              time.Duration
	PollMaxAfter         time.Duration
}

// ForControllerRuntime returns default controller-runtime options. Its basically just an alias.
func (co CrossplaneOptions) ForControllerRuntime() crcontroller.Options {
	return co.Options.ForControllerRuntime()
}

// ForControllerRuntimeWithBackoff returns controller-runtime options with an exponential backoff rate limiter.
func (co CrossplaneOptions) ForControllerRuntimeWithBackoff() crcontroller.Options {
	return withBackoff(co.MaxConcurrentReconciles, co.BackoffBase, co.BackoffMax)
}

var _ RuntimeOptionsGenerator = UpjetOptions{}

// UpjetOptions is a wrapper for adding more configuration on top of upjet controller options
type UpjetOptions struct {
	tjcontroller.Options

	BackoffBase          time.Duration
	BackoffMax           time.Duration
	PollJitterPercentage uint
	PollMax              time.Duration
	PollMaxAfter         time.Duration
}

// ForControllerRuntime returns default controller-runtime options. Its basically just an alias.
func (co UpjetOptions) ForControllerRuntime() crcontroller.Options {
	return co.Options.ForControllerRuntime()
}

// ForControllerRuntimeWithBackoff returns controller-runtime options with an exponential backoff rate limiter.
func (co UpjetOptions) ForControllerRuntimeWithBackoff() crcontroller.Options {
	return withBackoff(co.MaxConcurrentReconciles, co.BackoffBase, co.BackoffMax)
}

// PollReconcilerOptions returns the provider-wide poll-interval floor and hook.
func (co CrossplaneOptions) PollReconcilerOptions() []managed.ReconcilerOption {
	return pollReconcilerOptions(co.pollConfig())
}

// PollReconcilerOptions returns the provider-wide poll-interval floor and hook.
func (co UpjetOptions) PollReconcilerOptions() []managed.ReconcilerOption {
	return pollReconcilerOptions(co.pollConfig())
}

// WithExternalConnector installs c with the poll-backoff drift reset.
func (co CrossplaneOptions) WithExternalConnector(c managed.ExternalConnector) managed.ReconcilerOption {
	return withExternalConnector(co.pollConfig(), c)
}

// WithExternalConnector installs c with the poll-backoff drift reset.
func (co UpjetOptions) WithExternalConnector(c managed.ExternalConnector) managed.ReconcilerOption {
	return withExternalConnector(co.pollConfig(), c)
}

func (co CrossplaneOptions) pollConfig() pollinterval.Config {
	return pollinterval.Config{Poll: co.PollInterval, PollMax: co.PollMax, PollMaxAfter: co.PollMaxAfter, JitterPercentage: co.PollJitterPercentage}
}

func (co UpjetOptions) pollConfig() pollinterval.Config {
	return pollinterval.Config{Poll: co.PollInterval, PollMax: co.PollMax, PollMaxAfter: co.PollMaxAfter, JitterPercentage: co.PollJitterPercentage}
}

// withBackoff replicates xp's default ForControllerRuntime(), adding back an exponential backoff rate limiter.
func withBackoff(maxConcurrentReconciles int, backoffBase, backoffMax time.Duration) crcontroller.Options {
	recoverPanic := true
	return crcontroller.Options{
		MaxConcurrentReconciles: maxConcurrentReconciles,
		RateLimiter:             workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](backoffBase, backoffMax),
		RecoverPanic:            &recoverPanic,
		// opt out of the priority queue introduced with controller-runtime v0.25.0
		UsePriorityQueue: ptr.To(false),
	}
}

// pollReconcilerOptions floors the poll-interval annotation and installs the
// adaptive poll hook.
func pollReconcilerOptions(c pollinterval.Config) []managed.ReconcilerOption {
	return []managed.ReconcilerOption{
		managed.WithMinPollInterval(pollinterval.MinPollInterval),
		managed.WithPollIntervalHook(pollinterval.NewHook(c)),
	}
}

// withExternalConnector adds the drift reset only when backoff is enabled, so
// default flags leave Synced timestamps exactly as before.
func withExternalConnector(c pollinterval.Config, ec managed.ExternalConnector) managed.ReconcilerOption {
	if c.BackoffEnabled() {
		ec = pollinterval.WithDriftReset(ec)
	}
	return managed.WithExternalConnector(ec)
}
