package providerconfig

import (
	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/sap/crossplane-provider-btp/internal/features"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

type ConnectorFn func(
	kube client.Client,
	usage resource.Tracker,
	resourcetracker tracking.ReferenceResolverTracker,
) managed.ExternalConnecter

// DefaultSetup supports the creation of a controller for a given managed resource type. Accepts any type that implements the ConnectorFn or KymaModuleConnectorFn signature.
// DEPRECAED: use DefaultSetupWithoutDefaultInitializer instead to not have the external-name default initializer added automatically (new external-name handling requires external-name to be empty not defaulted on create).
func DefaultSetup(mgr ctrl.Manager, o controller.Options, object client.Object, kind string, gvk schema.GroupVersionKind, connectorFn ConnectorFn) error {
	name := managed.ControllerName(kind)

	referenceTracker := tracking.NewDefaultReferenceResolverTracker(
		mgr.GetClient(),
	)
	usageTracker :=
		resource.NewProviderConfigUsageTracker(
			mgr.GetClient(),
			&providerv1alpha1.ProviderConfigUsage{},
		)

	r := managed.NewReconciler(
		mgr,
		resource.ManagedKind(gvk),
		managed.WithExternalConnecter(connectorFn(mgr.GetClient(), usageTracker, referenceTracker)),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithPollInterval(o.PollInterval),
		connectionPublishers(mgr, o),
		enableBetaManagementPolicies(o.Features.Enabled(features.EnableBetaManagementPolicies)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(object).
		WithEventFilter(resource.DesiredStateChanged()).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// DefaultSetupWithoutDefaultInitializer works like DefaultSetup but without DefaultInitializer (to adhere to new external-name handling). It supports the creation of a controller for a given managed resource type. Accepts any type that implements the ConnectorFn or KymaModuleConnectorFn signature.
func DefaultSetupWithoutDefaultInitializer(mgr ctrl.Manager, o controller.Options, object client.Object, kind string, gvk schema.GroupVersionKind, connectorFn ConnectorFn) error {
	name := managed.ControllerName(kind)

	referenceTracker := tracking.NewDefaultReferenceResolverTracker(
		mgr.GetClient(),
	)
	usageTracker :=
		resource.NewProviderConfigUsageTracker(
			mgr.GetClient(),
			&providerv1alpha1.ProviderConfigUsage{},
		)

	r := managed.NewReconciler(
		mgr,
		resource.ManagedKind(gvk),
		managed.WithExternalConnecter(connectorFn(mgr.GetClient(), usageTracker, referenceTracker)),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithPollInterval(o.PollInterval),
		managed.WithInitializers(), // No default initializer
		connectionPublishers(mgr, o),
		enableBetaManagementPolicies(o.Features.Enabled(features.EnableBetaManagementPolicies)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(object).
		WithEventFilter(resource.DesiredStateChanged()).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

func connectionPublishers(mgr ctrl.Manager, o controller.Options) managed.ReconcilerOption {
	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), providerv1alpha1.StoreConfigGroupVersionKind))
	}
	return managed.WithConnectionPublishers(cps...)
}

func enableBetaManagementPolicies(enable bool) managed.ReconcilerOption {
	return func(r *managed.Reconciler) {
		if enable {
			managed.WithManagementPolicies()(r)
		}
	}
}
