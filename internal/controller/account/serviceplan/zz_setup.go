package serviceplan

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Setup adds a controller that reconciles ServicePlan managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	return providerconfig.DefaultSetup(mgr, o, &v1alpha1.ServicePlan{}, v1alpha1.ServicePlanKind, v1alpha1.ServicePlanGroupVersionKind, func(kube client.Client, usage resource.Tracker, resourcetracker tracking.ReferenceResolverTracker, newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)) managed.ExternalConnecter {
		return &connector{
			kube: mgr.GetClient(),
			usage: resource.NewProviderConfigUsageTracker(
				mgr.GetClient(),
				&providerv1alpha1.ProviderConfigUsage{},
			),
			resourcetracker: resourcetracker,
			newPlanIdResolverFn: func(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error) {
				binding, err := servicemanager.NewCredsFromOperatorSecret(secretData)
				if err != nil {
					return nil, err
				}
				return servicemanager.NewServiceManagerClient(ctx, &binding)
			},
		}
	})
}
