package serviceinstance

import (
	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/yaml"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/client/btpcli"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func convertServiceInstanceResource(si *btpcli.ServiceInstance) *yaml.ResourceWithComment {
	serviceInstance := yaml.NewResourceWithComment(
		&v1alpha1.ServiceInstance{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha1.ServiceInstanceKind,
				APIVersion: v1alpha1.CRDGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: si.Name + "-" + si.ID,
				Annotations: map[string]string{
					"crossplane.io/external-name": si.ID,
				},
			},
			Spec: v1alpha1.ServiceInstanceSpec{
				ResourceSpec: v1.ResourceSpec{
					ManagementPolicies: []v1.ManagementAction{
						v1.ManagementActionObserve,
					},
				},
				ForProvider: v1alpha1.ServiceInstanceParameters{
					Name: si.Name,
					// TODO: Parameters???
					OfferingName: "cis",   // TODO: remove hardcoded value
					PlanName:     "local", // TODO: remove hardcoded value
					SubaccountID: &si.SubaccountID,
				},
			},
		})
	return serviceInstance
}
