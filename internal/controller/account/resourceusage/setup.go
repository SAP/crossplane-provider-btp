package resourceusage

import (
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/providerconfig"

	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"
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
func Setup(mgr ctrl.Manager, o internalopts.CrossplaneOptions) error {
	name := providerconfig.ControllerName(v1alpha1.ResourceUsageGroupKind)

	r := NewReconciler(
		mgr,
		WithLogger(o.Logger.WithValues("controller", name)),
		WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntimeWithBackoff()).
		For(&v1alpha1.ResourceUsage{}).
		Watches(&v1alpha1.ResourceUsage{}, &EnqueueRequestForResourceUsage{}).
		WithEventFilter(resource.DesiredStateChanged()).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}
