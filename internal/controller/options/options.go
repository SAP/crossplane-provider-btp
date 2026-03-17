// Package options provides controller options helpers for controllers in this provider.
package options

import (
	"time"

	xpcontroller "github.com/crossplane/crossplane-runtime/pkg/controller"
	tjcontroller "github.com/crossplane/upjet/pkg/controller"
	"k8s.io/client-go/util/workqueue"
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

	BackoffBase time.Duration
	BackoffMax  time.Duration
}

// ForControllerRuntime returns default controller-runtime options. Its basically just an alias.
func (co CrossplaneOptions) ForControllerRuntime() crcontroller.Options {
	return co.Options.ForControllerRuntime()
}

// ForControllerRuntimeWithBackoff returns controller-runtime options with an exponential backoff rate limiter.
func (co CrossplaneOptions) ForControllerRuntimeWithBackoff() crcontroller.Options {
	// this is essentially replicated from xp default ForControllerRuntime() function. It should only add back configuration
	recoverPanic := true
	return crcontroller.Options{
		MaxConcurrentReconciles: co.MaxConcurrentReconciles,
		RateLimiter:             workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](co.BackoffBase, co.BackoffMax),
		RecoverPanic:            &recoverPanic,
	}
}

var _ RuntimeOptionsGenerator = UpjetOptions{}

// UpjetOptions is a wrapper for adding more configuration on top of upjet controller options
type UpjetOptions struct {
	tjcontroller.Options

	BackoffBase time.Duration
	BackoffMax  time.Duration
}

// ForControllerRuntime returns default controller-runtime options. Its basically just an alias.
func (co UpjetOptions) ForControllerRuntime() crcontroller.Options {
	return co.Options.ForControllerRuntime()
}

// ForControllerRuntimeWithBackoff returns controller-runtime options with an exponential backoff rate limiter.
func (co UpjetOptions) ForControllerRuntimeWithBackoff() crcontroller.Options {
	// this is essentially replicated from xp default ForControllerRuntime() function. It should only add back configuration
	recoverPanic := true
	return crcontroller.Options{
		MaxConcurrentReconciles: co.MaxConcurrentReconciles,
		RateLimiter:             workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](co.BackoffBase, co.BackoffMax),
		RecoverPanic:            &recoverPanic,
	}
}
