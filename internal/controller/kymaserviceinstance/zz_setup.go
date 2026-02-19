package kymaserviceinstance

import (
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymaserviceinstance"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

// Setup adds a controller that reconciles KymaServiceInstance managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	return providerconfig.DefaultSetup(
		mgr,
		o,
		&v1alpha1.KymaServiceInstance{},
		v1alpha1.KymaServiceInstanceKind,
		v1alpha1.KymaServiceInstanceGroupVersionKind,
		func(kube client.Client, usage resource.Tracker, resourcetracker tracking.ReferenceResolverTracker) managed.ExternalConnecter {
			return &connector{
				kube:            kube,
				usage:           usage,
				resourcetracker: resourcetracker,
				newServiceFn:    kymaserviceinstance.NewKymaServiceInstanceClient,
			}
		},
	)
}
