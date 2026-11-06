package subscription

import (
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

func convertSubscriptionResource(s *Subscription) *yaml.ResourceWithComment {
	resourceName := s.GenerateK8sResourceName()
	externalName := s.GetExternalName()

	subscription := yaml.NewResourceWithComment(&v1alpha1.Subscription{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.SubscriptionKind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: resourceName,
			Annotations: map[string]string{
				"crossplane.io/external-name": externalName,
			},
		},
		Spec: v1alpha1.SubscriptionSpec{
			ClusterManagedResourceSpec: xpv1.ClusterManagedResourceSpec{
				ManagementPolicies: []xpv1.ManagementAction{xpv1.ManagementActionObserve},
			},
			ForProvider: v1alpha1.SubscriptionParameters{
				AppName:                s.AppName,
				PlanName:               s.PlanName,
				SubscriptionParameters: runtime.RawExtension{Raw: s.Parameters},
			},
			// The controller resolves both secret coordinates from CloudManagement.
			// Its credentials also determine the subscription's subaccount.
			CloudManagementRef: &xpv1.Reference{Name: s.CloudManagementName},
		},
	})
	subscription.CloneComment(s)

	if s.SubaccountID == "" {
		subscription.AddComment(resources.WarnMissingSubaccountGuid)
	}
	if resourceName == resources.UndefinedName {
		subscription.AddComment(resources.WarnUndefinedResourceName)
	}
	if externalName == resources.UndefinedExternalName {
		subscription.AddComment(resources.WarnUndefinedExternalName)
	}
	if s.CloudManagementName == "" {
		subscription.AddComment(resources.WarnMissingCloudManagementName)
	}
	return subscription
}
