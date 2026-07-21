package servicemanager

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	apisv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Setup adds a controller that reconciles GlobalAccount managed resources.
func Setup(mgr ctrl.Manager, o internalopts.CrossplaneOptions) error {
	controllerName := managed.ControllerName(apisv1beta1.ServiceManagerKind)
	recorder := event.NewAPIRecorder(mgr.GetEventRecorderFor(controllerName)) //nolint:staticcheck // NewAPIRecorder requires the legacy event recorder type.
	return providerconfig.DefaultSetup(
		mgr,
		o,
		&apisv1beta1.ServiceManager{},
		apisv1beta1.ServiceManagerKind,
		apisv1beta1.ServiceManagerGroupVersionKind,
		func(kube client.Client,
			usage providerconfig.LegacyTracker,
			resourcetracker tracking.ReferenceResolverTracker) managed.ExternalConnector {
			return &connector{
				kube:            kube,
				newServiceFn:    btp.NewBTPClient,
				resourcetracker: resourcetracker,

				newPlanIdInitializerFn: func(ctx context.Context, cr *apisv1beta1.ServiceManager) (ServiceManagerPlanIdInitializer, error) {
					btpclient, err := providerconfig.CreateClient(ctx, cr, mgr.GetClient(), usage, btp.NewBTPClient, resourcetracker)
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

				newAdminLookuperFn: func(ctx context.Context, cr *apisv1beta1.ServiceManager) (servicemanager.SemanticLookuper, func(), error) {
					btpclient, err := providerconfig.CreateClient(ctx, cr, mgr.GetClient(), usage, btp.NewBTPClient, resourcetracker)
					if err != nil {
						return nil, func() {}, err
					}
					proxy := servicemanager.NewServiceManagerInstanceProxyClient(btpclient.AccountsServiceClient)
					return proxy.EnsureSemanticLookuper(ctx, cr.Spec.ForProvider.SubaccountGuid)
				},
				recorder: recorder,
			}
		})
}
