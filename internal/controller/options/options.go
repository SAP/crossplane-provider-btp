// Package options provides controller options helpers for controllers in this provider.
package options

import (
	"time"

	xpcontroller "github.com/crossplane/crossplane-runtime/pkg/controller"
	"k8s.io/client-go/util/workqueue"
	crcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// maxExponentialBackoff is the maximum delay for the per-item exponential
// backoff rate limiter used by all controllers in this provider.
const maxExponentialBackoff = 120 * time.Second

// ForControllerRuntime returns controller-runtime options equivalent to
// [xpcontroller.Options.ForControllerRuntime], but with a maximum exponential
// backoff of [maxExponentialBackoff] instead of the upstream default of 60s.
func ForControllerRuntime(o xpcontroller.Options) crcontroller.Options {
	recoverPanic := true
	return crcontroller.Options{
		MaxConcurrentReconciles: o.MaxConcurrentReconciles,
		RateLimiter:             workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, maxExponentialBackoff),
		RecoverPanic:            &recoverPanic,
	}
}
