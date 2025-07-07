package resourceusage

import (
	"context"
	"time"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

const (
	shortWait = 30 * time.Second
	timeout   = 2 * time.Minute

	errListPCUs     = "cannot list ResourceUsages"
	errDeletePU     = "cannot delete ResourceUsage"
	errUpdate       = "cannot update ResourceUsage"
	errUpdateStatus = "cannot update ResourceUsage status"
)

// Event reasons.
const (
	reasonAccount event.Reason = "UsageAccounting"
)

// Condition types and reasons.
const (
	TypeTerminating xpv1.ConditionType   = "Terminating"
	ReasonInUse     xpv1.ConditionReason = "InUse"
)

// Terminating indicates a ResourceUsage has been deleted, but that the
// deletion is being blocked because it is still in use.
func Terminating() xpv1.Condition {
	return xpv1.Condition{
		Type:               TypeTerminating,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonInUse,
	}
}

// ControllerName returns the recommended name for controllers that use this
// package to reconcile a particular kind of managed resource.
func ControllerName() string {
	return "orchestrate.cloud.sap/resourceusage"
}

// A Reconciler reconciles managed resources by creating and managing the
// lifecycle of an external resource, i.e. a resource in an external system such
// as a cloud provider API. Each controller must watch the managed resource kind
// for which it is responsible.
type Reconciler struct {
	client  client.Client
	tracker tracking.ReferenceResolverTracker

	newConfig func() v1alpha1.ResourceUsage
	log       logging.Logger
	record    event.Recorder
}

// A ReconcilerOption configures a Reconciler.
type ReconcilerOption func(*Reconciler)

// WithLogger specifies how the Reconciler should log messages.
func WithLogger(l logging.Logger) ReconcilerOption {
	return func(r *Reconciler) {
		r.log = l
	}
}

// WithRecorder specifies how the Reconciler should record events.
func WithRecorder(er event.Recorder) ReconcilerOption {
	return func(r *Reconciler) {
		r.record = er
	}
}

// NewReconciler returns a Reconciler of ProviderConfigs.
func NewReconciler(m manager.Manager, o ...ReconcilerOption) *Reconciler {
	nc := func() v1alpha1.ResourceUsage {
		return v1alpha1.ResourceUsage{}
	}
	nul := func() v1alpha1.ResourceUsageList {
		return v1alpha1.ResourceUsageList{}
	}

	// Panic early if we've been asked to reconcile a resource kind that has not
	// been registered with our controller manager's scheme.
	_, _ = nc(), nul()

	r := &Reconciler{
		client:    m.GetClient(),
		tracker:   tracking.NewDefaultReferenceResolverTracker(m.GetClient()),
		newConfig: nc,

		log:    logging.NewNopLogger(),
		record: event.NewNopRecorder(),
	}

	for _, ro := range o {
		ro(r)
	}

	return r
}

// Reconcile a ResourceUsage by accounting for the managed resources that are
// using it, and ensuring it cannot be deleted until it is no longer in use.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("request", req)
	log.Debug("Reconciling")

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ru := lo.ToPtr(r.newConfig())
	if err := r.client.Get(ctx, req.NamespacedName, ru); err != nil {
		// In case object is not found, most likely the object was deleted and
		// then disappeared while the event was in the processing queue. We
		// don't need to take any action in that case.
		log.Debug(errGetPC, "error", err)
		return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), errGetPC)
	}

	log = log.WithValues(
		"uid", ru.GetUID(),
		"version", ru.GetResourceVersion(),
		"name", ru.GetName(),
	)
	// check if target is still in place (on delete)
	target, err := r.tracker.ResolveTarget(ctx, *ru)
	if resource.IgnoreNotFound(err) != nil {
		return reconcile.Result{}, err
	}
	// target has already been deleted
	if kerrors.IsNotFound(err) && target == nil {
		meta.RemoveFinalizer(ru, v1alpha1.Finalizer)
		if err := r.client.Update(ctx, ru); err != nil {
			r.log.Debug(errUpdate, "error", err)
			return reconcile.Result{RequeueAfter: shortWait}, nil
		}
		if err := r.client.Delete(ctx, ru); err != nil {
			r.log.Debug(errDeletePU, "error", err)
			return reconcile.Result{RequeueAfter: shortWait}, nil
		}
		return reconcile.Result{Requeue: false}, nil
	}

	if meta.WasDeleted(ru) {
		if target != nil {
			msg := "Blocking deletion while target still exist"

			log.Debug(msg)
			r.record.Event(ru, event.Warning(reasonAccount, errors.New(msg)))

			// We're watching our usages, so we'll be requeued when they go.
			return reconcile.Result{Requeue: false}, nil
		}
		// Deletion and removal of finalizer must happen before
		return reconcile.Result{Requeue: true}, errors.New("inconsistent state, do requeue")
	}
	if !meta.FinalizerExists(ru, v1alpha1.Finalizer) {
		meta.AddFinalizer(ru, v1alpha1.Finalizer)
		if err := r.client.Update(ctx, ru); err != nil {
			r.log.Debug(errUpdate, "error", err)
			return reconcile.Result{RequeueAfter: shortWait}, nil
		}
	}

	return reconcile.Result{Requeue: false}, nil
}
