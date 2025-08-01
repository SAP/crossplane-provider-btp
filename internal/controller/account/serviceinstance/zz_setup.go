package serviceinstance

import (
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrl "sigs.k8s.io/controller-runtime"
)

// Setup adds a controller that reconciles ServiceInstance managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	return providerconfig.DefaultSetup(mgr, o, &v1alpha1.ServiceInstance{}, v1alpha1.ServiceInstanceGroupKind, v1alpha1.ServiceInstanceGroupVersionKind, func(kube client.Client, usage resource.Tracker, resourcetracker tracking.ReferenceResolverTracker) managed.ExternalConnecter {
		return &connector{
			kube:  kube,
			usage: usage,

			newServicePlanInitializerFn: newServicePlanInitializerFn,

			// instead of passing the creatorFn as usual we need to execute here to make sure the connector has only one instance of the client
			// this is required to ensure terraform workspace is shared among reconciliation loops, since the state of async operations is stored in the client
			clientConnector: newClientCreatorFn(mgr.GetClient()),
		}
	})
}
