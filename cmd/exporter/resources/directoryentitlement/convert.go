package directoryentitlement

import (
	"context"

	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/erratt"
	"github.com/SAP/xp-clifford/yaml"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/directory"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func convertDirectoryEntitlementResource(ctx context.Context, btpClient *btpcli.BtpCli, e *directoryEntitlement, handler export.EventHandler, resolveReferences bool) *yaml.ResourceWithComment {
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
	if resolveReferences {
		if err := resolveReference(ctx, btpClient, &cr.Spec.ForProvider); err != nil {
			handler.Warn(erratt.Errorf("cannot resolve directory reference: %w", err).With("entitlement", e.GetID()))
			result.AddComment(resources.WarnCannotResolveDirectory + ": " + e.directoryID)
		}
	}
	return result
}

func resolveReference(ctx context.Context, btpClient *btpcli.BtpCli, spec *v1alpha1.DirectoryEntitlementParameters) error {
	name, err := directory.GetK8sResourceNameByID(ctx, btpClient, ptr.Deref(spec.DirectoryID, ""))
	if err != nil {
		return err
	}
	spec.DirectoryRef = &xpv1.Reference{Name: name}
	spec.DirectoryID = nil
	return nil
}
