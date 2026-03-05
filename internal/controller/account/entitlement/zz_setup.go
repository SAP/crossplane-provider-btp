package entitlement

import (
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	entitlementclient "github.com/sap/crossplane-provider-btp/internal/clients/entitlement"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

// Setup adds a controller that reconciles Entitlement managed resources.
func Setup(mgr ctrl.Manager, o controller.Options, cacheTTL time.Duration) error {
	cache := entitlementclient.NewInstanceCache(cacheTTL)
	entitlementclient.RegisterCacheMetrics(cache, metrics.Registry)
	return providerconfig.DefaultSetup(mgr, o, &apisv1alpha1.Entitlement{}, apisv1alpha1.EntitlementGroupKind, apisv1alpha1.EntitlementGroupVersionKind, func(kube client.Client, usage resource.Tracker, resourcetracker tracking.ReferenceResolverTracker) managed.ExternalConnecter {
		return &connector{
			kube:            kube,
			usage:           usage,
			resourcetracker: resourcetracker,
			newServiceFn:    btp.NewBTPClient,
			cache:           cache,
		}
	})
}
