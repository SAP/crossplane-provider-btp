package controller

import (
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/servicebinding"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/serviceinstance"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/sap/crossplane-provider-btp/internal/controller/account/cloudmanagement"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/directory"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/entitlement"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/globalaccount"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/resourceusage"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/subaccount"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/subscription"
	"github.com/sap/crossplane-provider-btp/internal/controller/environment/cloudfoundry"
	"github.com/sap/crossplane-provider-btp/internal/controller/environment/kyma"
	"github.com/sap/crossplane-provider-btp/internal/controller/kymaenvironmentbinding"
	"github.com/sap/crossplane-provider-btp/internal/controller/oidc/certbasedoidclogin"
	"github.com/sap/crossplane-provider-btp/internal/controller/oidc/kubeconfiggenerator"
	"github.com/sap/crossplane-provider-btp/internal/controller/security/rolecollection"
	"github.com/sap/crossplane-provider-btp/internal/controller/security/rolecollectionassignment"
)

// CustomSetup creates all Template controllers with the supplied logger and adds them to
// the supplied manager.
func CustomSetup(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		globalaccount.Setup,
		subaccount.Setup,
		cloudfoundry.Setup,
		kyma.Setup,
		entitlement.Setup,
		cloudmanagement.Setup,
		servicemanager.Setup,
		resourceusage.Setup,
		certbasedoidclogin.Setup,
		kubeconfiggenerator.Setup,
		directory.Setup,
		subscription.Setup,
		rolecollectionassignment.Setup,
		rolecollection.Setup,
		serviceinstance.Setup,
		servicebinding.Setup,
		kymaenvironmentbinding.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}
