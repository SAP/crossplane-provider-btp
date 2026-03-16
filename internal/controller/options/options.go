// Package options provides controller options helpers for controllers in this provider.
package options

import (
	"time"

	xpcontroller "github.com/crossplane/crossplane-runtime/pkg/controller"
	"k8s.io/client-go/util/workqueue"
	crcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Options is a wrapper for merging controller specific options to generic xp controller options.
type Options struct {
	xpcontroller.Options
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

// ForControllerRuntime returns default controller-runtime options. Its basically just an alias.
func ForControllerRuntime(o xpcontroller.Options) crcontroller.Options {
	return o.ForControllerRuntime()
}

// ForControllerRuntimeWithBackoff returns controller-runtime options with an exponential backoff rate limiter.
func ForControllerRuntimeWithBackoff(o Options) crcontroller.Options {
	recoverPanic := true
	return crcontroller.Options{
		MaxConcurrentReconciles: o.MaxConcurrentReconciles,
		RateLimiter:             workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](o.BaseBackoff, o.MaxBackoff),
		RecoverPanic:            &recoverPanic,
	}
}
