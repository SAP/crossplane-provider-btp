package cloudmanagement

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/sap/crossplane-provider-btp/btp"
	cmClient "github.com/sap/crossplane-provider-btp/internal/clients/cis"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"github.com/sap/crossplane-provider-btp/internal/di"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	apisv1beta1 "github.com/sap/crossplane-provider-btp/apis/account/v1beta1"

	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

// Setup adds a controller that reconciles CloudManagement managed resources.
func Setup(mgr ctrl.Manager, o internalopts.CrossplaneOptions) error {
	name := managed.ControllerName(apisv1beta1.CloudManagementKind)
	recorder := event.NewAPIRecorder(mgr.GetEventRecorderFor(name)) //nolint:staticcheck // NewAPIRecorder requires the legacy event recorder type.
	return providerconfig.DefaultSetup(mgr, o, &apisv1beta1.CloudManagement{}, apisv1beta1.CloudManagementKind, apisv1beta1.CloudManagementGroupVersionKind, func(kube client.Client, usage providerconfig.LegacyTracker, resourcetracker tracking.ReferenceResolverTracker) managed.ExternalConnector {
		return &connector{
			kube:                kube,
			usage:               usage,
			resourcetracker:     resourcetracker,
			newPlanIdResolverFn: di.NewPlanIdResolverFn,

			newClientInitalizerFn: func() cmClient.ITfClientInitializer {
				return cmClient.NewTfClient(
					tfclient.NewInternalTfConnector(mgr.GetClient(), "btp_subaccount_service_instance", apisv1alpha1.SubaccountServiceInstance_GroupVersionKind, false, nil),
					tfclient.NewInternalTfConnector(mgr.GetClient(), "btp_subaccount_service_binding", apisv1alpha1.SubaccountServiceBinding_GroupVersionKind, false, nil),
				)
			},

			// Adoption uses the subaccount-admin SM binding (via the accounts-service),
			// keyed by the CM's subaccountGuid — not the platform-scoped serviceManagerSecret.
			newAdminLookuperFn: func(ctx context.Context, cr *apisv1beta1.CloudManagement) (smClient.SemanticLookuper, func(), error) {
				noop := func() {}
				btpClient, err := providerconfig.CreateClient(ctx, cr, mgr.GetClient(), usage, btp.NewBTPClient, resourcetracker)
				if err != nil {
					return nil, noop, err
				}
				proxy := smClient.NewServiceManagerInstanceProxyClient(btpClient.AccountsServiceClient)
				return proxy.EnsureSemanticLookuper(ctx, cr.Spec.ForProvider.SubaccountGuid)
			},
			recorder: recorder,
		}
	})
}
