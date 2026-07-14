package servicebinding

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

// Setup adds a controller that reconciles ServiceBinding managed resources.
func Setup(mgr ctrl.Manager, o internalopts.CrossplaneOptions) error {
	name := managed.ControllerName(v1alpha1.ServiceBindingGroupKind)
	recorder := event.NewAPIRecorder(mgr.GetEventRecorderFor(name)) //nolint:staticcheck // NewAPIRecorder requires the legacy event recorder type.
	return providerconfig.DefaultSetup(mgr, o, &v1alpha1.ServiceBinding{}, v1alpha1.ServiceBindingGroupKind, v1alpha1.ServiceBindingGroupVersionKind, func(kube client.Client, usage providerconfig.LegacyTracker, resourcetracker tracking.ReferenceResolverTracker) managed.ExternalConnector {
		tfConnector := newTfConnectorFn(kube)
		return &connector{
			kube:              kube,
			usage:             usage,
			resourcetracker:   resourcetracker,
			clientFactory:     newServiceBindingClientFactory(kube, tfConnector),
			newSBKeyRotatorFn: newSBKeyRotatorFn,

			// Adoption uses the subaccount-admin SM binding (via the accounts-service),
			// keyed by the binding's own subaccountId — not the parent SI's
			// serviceManagerSecret (platform-scoped, cannot list managed instances).
			newAdminLookuperFn: func(ctx context.Context, cr *v1alpha1.ServiceBinding) (smClient.SemanticLookuper, func(), error) {
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
