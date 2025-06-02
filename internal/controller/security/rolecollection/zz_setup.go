package rolecollection

import (
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

// Setup adds a controller that reconciles RoleCollection managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	return providerconfig.DefaultSetup(mgr, o, &v1alpha1.RoleCollection{}, v1alpha1.RoleCollectionGroupKind, v1alpha1.RoleCollectionGroupVersionKind, func(kube client.Client, usage resource.Tracker, resourcetracker tracking.ReferenceResolverTracker, newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)) managed.ExternalConnecter {
		return &connector{
			kube:            mgr.GetClient(),
			usage:           resource.NewProviderConfigUsageTracker(mgr.GetClient(), &providerv1alpha1.ProviderConfigUsage{}),
			newServiceFn:    configureRoleCollectionMaintainerFn,
			resourcetracker: resourcetracker,
		}
	})
}
