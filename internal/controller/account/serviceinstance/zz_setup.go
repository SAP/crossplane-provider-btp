package serviceinstance

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrl "sigs.k8s.io/controller-runtime"
)

// Setup adds a controller that reconciles ServiceInstance managed resources.
func Setup(mgr ctrl.Manager, o internalopts.CrossplaneOptions) error {
	name := managed.ControllerName(v1alpha1.ServiceInstanceGroupKind)
	recorder := event.NewAPIRecorder(mgr.GetEventRecorderFor(name)) //nolint:staticcheck // NewAPIRecorder requires the legacy event recorder type.
	return providerconfig.DefaultSetupWithoutDefaultInitializer(mgr, o, &v1alpha1.ServiceInstance{}, v1alpha1.ServiceInstanceGroupKind, v1alpha1.ServiceInstanceGroupVersionKind, func(kube client.Client, usage providerconfig.LegacyTracker, resourcetracker tracking.ReferenceResolverTracker) managed.ExternalConnector {
		return &connector{
			kube:  kube,
			usage: usage,

			newServicePlanInitializerFn: newServicePlanInitializerFn,

			// instead of passing the creatorFn as usual we need to execute here to make sure the connector has only one instance of the client
			// this is required to ensure terraform workspace is shared among reconciliation loops, since the state of async operations is stored in the client
			clientConnector: newClientCreatorFn(mgr.GetClient()),
			resourcetracker: resourcetracker,

			// Adoption uses the subaccount-admin SM binding (via the accounts-service),
			// NOT the per-resource serviceManagerSecret (which is platform-scoped and
			// cannot see instances created by the btp terraform provider).
			newAdminLookuperFn: func(ctx context.Context, cr *v1alpha1.ServiceInstance) (smClient.SemanticLookuper, func(), error) {
				noop := func() {}
				btpClient, err := providerconfig.CreateClient(ctx, cr, mgr.GetClient(), usage, btp.NewBTPClient, resourcetracker)
				if err != nil {
					return nil, noop, err
				}
				proxy := smClient.NewServiceManagerInstanceProxyClient(btpClient.AccountsServiceClient)
				return proxy.EnsureSemanticLookuper(ctx, internal.Val(cr.Spec.ForProvider.SubaccountID))
			},
			recorder: recorder,
		}
	})
}
