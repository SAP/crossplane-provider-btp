package directoryentitlement

import (
	"github.com/SAP/xp-clifford/yaml"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func convertDirectoryEntitlementResource(e *directoryEntitlement) *yaml.ResourceWithComment {
	parameters := v1alpha1.DirectoryEntitlementParameters{
		DirectoryID: ptr.To(e.directoryID),
		ServiceName: ptr.To(e.serviceName),
		PlanName:    ptr.To(e.plan.Name),
		AutoAssign:  ptr.To(e.plan.AutoAssign),
	}
	if e.plan.UniqueIdentifier != "" {
		parameters.PlanUniqueIdentifier = ptr.To(e.plan.UniqueIdentifier)
	}
	//  Only set quota for numeric plan categories.
	switch e.plan.Category {
	case "SERVICE", "QUOTA_BASED_APPLICATION", "PLATFORM", "ENVIRONMENT":
		parameters.Amount = e.plan.Amount
		parameters.AutoDistributeAmount = e.plan.AutoDistributeAmount
	}
	cr := &v1alpha1.DirectoryEntitlement{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.DirectoryEntitlement_Kind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        e.GenerateK8sResourceName(),
			Annotations: map[string]string{"crossplane.io/external-name": e.GetExternalName()},
		},
		Spec: v1alpha1.DirectoryEntitlementSpec{
			ClusterManagedResourceSpec: xpv1.ClusterManagedResourceSpec{
				ManagementPolicies: []xpv1.ManagementAction{xpv1.ManagementActionObserve},
			},
			ForProvider: parameters,
		},
	}
	result := yaml.NewResourceWithComment(cr)
	result.CloneComment(e)
	if e.serviceName == "" {
		result.AddComment(resources.WarnMissingServiceName)
	}
	if e.plan.Name == "" {
		result.AddComment(resources.WarnMissingServicePlanName)
	}
	if e.directoryID == "" {
		result.AddComment(resources.WarnMissingDirectoryGuid)
	}
	if cr.Name == resources.UndefinedName {
		result.AddComment(resources.WarnUndefinedResourceName)
	}
	if e.GetExternalName() == resources.UndefinedExternalName {
		result.AddComment(resources.WarnUndefinedExternalName)
	}
	return result
}
