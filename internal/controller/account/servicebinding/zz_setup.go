package servicebinding

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

// Setup adds a controller that reconciles ServiceBinding managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	return providerconfig.DefaultSetup(mgr, o, &v1alpha1.ServiceBinding{}, v1alpha1.ServiceBindingGroupKind, v1alpha1.ServiceBindingGroupVersionKind, func(kube client.Client, usage resource.Tracker, resourcetracker tracking.ReferenceResolverTracker) managed.ExternalConnecter {
		return &connector{
			kube:            kube,
			usage:           usage,
			resourcetracker: resourcetracker,
			tfConnector:               newTfConnectorFn(kube),
			newServiceBindingClientFn: newServiceBindingClientFn,
			newSBKeyRotatorFn:        newSBKeyRotatorFn,
		}
	})
}
