package cloudmanagement

import (
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	cmClient "github.com/sap/crossplane-provider-btp/internal/clients/cis"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"github.com/sap/crossplane-provider-btp/internal/di"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/controller/providerconfig"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

// Setup adds a controller that reconciles CloudManagement managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	return providerconfig.DefaultSetup(mgr, o, &apisv1alpha1.CloudManagement{}, apisv1alpha1.CloudManagementKind, apisv1alpha1.CloudManagementGroupVersionKind, func(kube client.Client, usage resource.Tracker, resourcetracker tracking.ReferenceResolverTracker, newServiceFn func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error)) managed.ExternalConnecter {
		return &connector{
			kube: mgr.GetClient(),
			usage: resource.NewProviderConfigUsageTracker(
				mgr.GetClient(),
				&providerv1alpha1.ProviderConfigUsage{},
			),
			resourcetracker:     resourcetracker,
			newPlanIdResolverFn: di.NewPlanIdResolverFn,

			newClientInitalizerFn: func() cmClient.ITfClientInitializer {
				return cmClient.NewTfClient(
					tfclient.NewInternalTfConnector(mgr.GetClient(), "btp_subaccount_service_instance", apisv1alpha1.SubaccountServiceInstance_GroupVersionKind, false, nil),
					tfclient.NewInternalTfConnector(mgr.GetClient(), "btp_subaccount_service_binding", apisv1alpha1.SubaccountServiceBinding_GroupVersionKind, false, nil),
				)
			},
		}
	})
}
