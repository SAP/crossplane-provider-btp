package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	internalopts "github.com/sap/crossplane-provider-btp/internal/controller/options"

	// Cluster-scoped controllers
	clusteraccount "github.com/sap/crossplane-provider-btp/internal/controller/cluster/account"
	clustersecurity "github.com/sap/crossplane-provider-btp/internal/controller/cluster/security"

	// Legacy non-scoped controllers
	"github.com/sap/crossplane-provider-btp/internal/controller/account/cloudmanagement"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/entitlement"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/resourceusage"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/servicebinding"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/serviceinstance"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/servicemanager"
	"github.com/sap/crossplane-provider-btp/internal/controller/account/subscription"
	"github.com/sap/crossplane-provider-btp/internal/controller/environment/cloudfoundry"
	"github.com/sap/crossplane-provider-btp/internal/controller/environment/kyma"
	"github.com/sap/crossplane-provider-btp/internal/controller/kymaenvironmentbinding"
	"github.com/sap/crossplane-provider-btp/internal/controller/kymamodule"
	"github.com/sap/crossplane-provider-btp/internal/controller/oidc/certbasedoidclogin"
	"github.com/sap/crossplane-provider-btp/internal/controller/oidc/kubeconfiggenerator"
	"github.com/sap/crossplane-provider-btp/internal/controller/security/rolecollectionassignment"
)

// CustomSetup creates all Template controllers with the supplied logger and adds them to
// the supplied manager.
func CustomSetup(mgr ctrl.Manager, o internalopts.CrossplaneOptions) error {
	for _, setup := range []func(ctrl.Manager, internalopts.CrossplaneOptions) error{
		clusteraccount.Setup,
		clustersecurity.Setup,
		cloudfoundry.Setup,
		kyma.Setup,
		entitlement.Setup,
		cloudmanagement.Setup,
		servicemanager.Setup,
		resourceusage.Setup,
		certbasedoidclogin.Setup,
		kubeconfiggenerator.Setup,
		subscription.Setup,
		rolecollectionassignment.Setup,
		serviceinstance.Setup,
		servicebinding.Setup,
		kymaenvironmentbinding.Setup,
		kymamodule.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}
