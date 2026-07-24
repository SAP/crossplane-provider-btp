package subaccount

import (
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

// Setup adds a controller that reconciles GlobalAccount managed resources.
func Setup(mgr ctrl.Manager, o internalopts.CrossplaneOptions) error {
	name := managed.ControllerName(apisv1alpha1.SubaccountGroupKind)
	recorder := event.NewAPIRecorder(mgr.GetEventRecorderFor(name)) //nolint:staticcheck // NewAPIRecorder requires the legacy event recorder type.
	return providerconfig.DefaultSetupWithoutDefaultInitializer(mgr, o, &apisv1alpha1.Subaccount{}, apisv1alpha1.SubaccountGroupKind, apisv1alpha1.SubaccountGroupVersionKind, func(kube client.Client, usage providerconfig.LegacyTracker, resourcetracker tracking.ReferenceResolverTracker) managed.ExternalConnector {
		return &connector{
			kube:            kube,
			usage:           usage,
			newServiceFn:    btp.NewBTPClient,
			resourcetracker: resourcetracker,
			recorder:        recorder,
		}
	})
}
