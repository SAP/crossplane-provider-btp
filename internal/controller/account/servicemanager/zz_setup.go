package servicemanager

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	apisv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Setup adds a controller that reconciles GlobalAccount managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	return providerconfig.DefaultSetup(
		mgr,
		o,
		&apisv1beta1.ServiceManager{},
		apisv1beta1.ServiceManagerKind,
		apisv1beta1.ServiceManagerGroupVersionKind,
		func(kube client.Client,
			usage resource.Tracker,
			resourcetracker tracking.ReferenceResolverTracker,
			newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)) managed.ExternalConnecter {
			tracker := resource.NewProviderConfigUsageTracker(
				mgr.GetClient(),
				&providerv1alpha1.ProviderConfigUsage{},
			)
			return &connector{
				kube:            mgr.GetClient(),
				newServiceFn:    btp.NewBTPClient,
				resourcetracker: resourcetracker,

				newPlanIdInitializerFn: func(ctx context.Context, cr *apisv1beta1.ServiceManager) (ServiceManagerPlanIdInitializer, error) {
					btpclient, err := providerconfig.CreateClient(ctx, cr, mgr.GetClient(), tracker, btp.NewBTPClient, resourcetracker)
					if err != nil {
						return nil, err
					}

					smInstanceClient := servicemanager.NewServiceManagerInstanceProxyClient(btpclient.AccountsServiceClient)
					return smInstanceClient, nil
				},

				newClientInitalizerFn: func() servicemanager.ITfClientInitializer {
					return servicemanager.NewServiceManagerTfClient(
						tfclient.NewInternalTfConnector(mgr.GetClient(), "btp_subaccount_service_instance", apisv1alpha1.SubaccountServiceInstance_GroupVersionKind, false, nil),
						tfclient.NewInternalTfConnector(mgr.GetClient(), "btp_subaccount_service_binding", apisv1alpha1.SubaccountServiceBinding_GroupVersionKind, false, nil),

						servicemanager.Defaults{
							InstanceName: apisv1beta1.DefaultServiceInstanceName,
							BindingName:  apisv1beta1.DefaultServiceBindingName,
						},
					)
				},
			}
		})
}
