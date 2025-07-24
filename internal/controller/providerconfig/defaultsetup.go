package providerconfig

import (
	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymamodule"
	"github.com/sap/crossplane-provider-btp/internal/features"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

type ConnectorFn func(
	kube client.Client,
	usage resource.Tracker,
	resourcetracker tracking.ReferenceResolverTracker,
	newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error),
) managed.ExternalConnecter

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
		managed.WithExternalConnecter(connectorFn(mgr.GetClient(), usageTracker, referenceTracker, btp.NewBTPClient)),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
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

type KymaModuleConnectorFn func(
	kube client.Client,
	usage resource.Tracker,
	resourcetracker tracking.ReferenceResolverTracker,
	newServiceFn func(dynamic dynamic.Interface) *kymamodule.KymaModuleClient,
) managed.ExternalConnecter

func DefaultKymaModuleSetup(
	mgr ctrl.Manager,
	o controller.Options,
	object client.Object,
	kind string,
	gvk schema.GroupVersionKind,
	connectorFn KymaModuleConnectorFn,
) error {
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
		managed.WithExternalConnecter(connectorFn(mgr.GetClient(), usageTracker, referenceTracker, kymamodule.NewKymaModuleClient)),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
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
