package servicebinding

import (
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

// Setup adds a controller that reconciles ServiceBinding managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	return providerconfig.DefaultSetup(mgr, o, &v1alpha1.ServiceBinding{}, v1alpha1.ServiceBindingGroupKind, v1alpha1.ServiceBindingGroupVersionKind, func(kube client.Client, usage resource.Tracker, resourcetracker tracking.ReferenceResolverTracker) managed.ExternalConnecter {
		tfConnector := newTfConnectorFn(kube)
		return &connector{
			kube:              kube,
			usage:             usage,
			resourcetracker:   resourcetracker,
			clientFactory:     newServiceBindingClientFactory(kube, tfConnector),
			newSBKeyRotatorFn: newSBKeyRotatorFn,
		}
	})
}
