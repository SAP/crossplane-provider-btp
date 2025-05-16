package serviceinstanceclient

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/tfclient"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewServiceInstanceConnector(saveConditionsCallback tfclient.SaveConditionsFn, kube client.Client) tfclient.TfProxyConnectorI[*v1alpha1.ServiceInstance] {
	con := &ServiceInstanceConnector{
		TfProxyConnector: tfclient.NewTfProxyConnector(
			tfclient.NewInternalTfConnector(
				kube,
				"btp_subaccount_service_instance",
				v1alpha1.SubaccountServiceInstance_GroupVersionKind,
				true,
				tfclient.NewAPICallbacks(
					kube,
					saveConditionsCallback,
				),
			),
			&ServiceInstanceMapper{},
		),
	}
	return con
}

type ServiceInstanceConnector struct {
	tfclient.TfProxyConnector[*v1alpha1.ServiceInstance, *v1alpha1.SubaccountServiceInstance]
}

type ServiceInstanceMapper struct {
}

func (s *ServiceInstanceMapper) TfResource(si *v1alpha1.ServiceInstance) *v1alpha1.SubaccountServiceInstance {
	sInstance := &v1alpha1.SubaccountServiceInstance{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.SubaccountServiceInstance_Kind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: si.Name,
			// make sure no naming conflicts are there for upjet tmp folder creation
			UID:               si.UID + "-service-instance",
			DeletionTimestamp: si.DeletionTimestamp,
		},
		Spec: v1alpha1.SubaccountServiceInstanceSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: pcName(si),
				},
				ManagementPolicies:               []xpv1.ManagementAction{xpv1.ManagementActionAll},
				WriteConnectionSecretToReference: si.GetWriteConnectionSecretToReference(),
			},
			ForProvider: v1alpha1.SubaccountServiceInstanceParameters{
				Name:          &si.Name,
				ServiceplanID: si.Spec.ForProvider.ServiceplanID,
				SubaccountID:  si.Spec.ForProvider.SubaccountID,
			},
			InitProvider: v1alpha1.SubaccountServiceInstanceInitParameters{},
		},
		Status: v1alpha1.SubaccountServiceInstanceStatus{},
	}
	meta.SetExternalName(sInstance, meta.GetExternalName(si))
	return sInstance
}

func pcName(si *v1alpha1.ServiceInstance) string {
	pc := si.GetProviderConfigReference()
	if pc != nil && pc.Name != "" {
		return pc.Name
	}
	return ""
}
