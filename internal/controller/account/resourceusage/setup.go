package resourceusage

import (
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/providerconfig"

	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	internallimiter "github.com/sap/crossplane-provider-btp/internal/ratelimiter"
)

const (
	errGetPC        = "cannot get ResourceUsage"
	errGetCreds     = "cannot get credentials"
	errTrackRUsage  = "cannot track ResourceUsage"
	errTrackPCUsage = "cannot track ResourceUsage usage"
	errNewClient    = "cannot create new Service"
)

// Setup adds a controller that reconciles ResourceUsages by accounting for
// their current usage.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := providerconfig.ControllerName(v1alpha1.ResourceUsageGroupKind)

	r := NewReconciler(
		mgr,
		WithLogger(o.Logger.WithValues("controller", name)),
		WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(internallimiter.ForControllerRuntime(o)).
		For(&v1alpha1.ResourceUsage{}).
		Watches(&v1alpha1.ResourceUsage{}, &EnqueueRequestForResourceUsage{}).
		WithEventFilter(resource.DesiredStateChanged()).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}
