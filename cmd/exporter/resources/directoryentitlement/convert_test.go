package directoryentitlement

import (
	"testing"

	"github.com/SAP/xp-clifford/yaml"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestConvertDirectoryEntitlementResource(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		category string
		numeric  bool
	}{
		{category: "SERVICE", numeric: true},
		{category: "QUOTA_BASED_APPLICATION", numeric: true},
		{category: "PLATFORM", numeric: true},
		{category: "ENVIRONMENT", numeric: true},
		{category: "ELASTIC_SERVICE"},
		{category: "ELASTIC_LIMITED"},
		{category: "APPLICATION"},
	} {
		t.Run(tt.category, func(t *testing.T) {
			e := &directoryEntitlement{
				directoryID: "dir-one", serviceName: "service",
				plan: btpcli.DirectoryEntitledServicePlan{
					Name: "standard", Category: tt.category, UniqueIdentifier: "plan-id",
					Amount: ptr.To(0.0), AutoAssign: true, AutoDistributeAmount: ptr.To(2.0),
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			}
			result := convertDirectoryEntitlementResource(e)
			cr := result.Resource().(*v1alpha1.DirectoryEntitlement)
			require.Equal(t, "DirectoryEntitlement", cr.Kind)
			require.Equal(t, v1alpha1.CRDGroupVersion.String(), cr.APIVersion)
			require.Equal(t, "dir-one/service/standard", cr.Annotations["crossplane.io/external-name"])
			require.Equal(t, "service.standard.dir-one", cr.Name)
			require.Equal(t, xpv1.ManagementPolicies{xpv1.ManagementActionObserve}, cr.Spec.ManagementPolicies)
			parameters := cr.Spec.ForProvider
			require.Equal(t, ptr.To("dir-one"), parameters.DirectoryID)
			require.Nil(t, parameters.DirectoryRef)
			require.Equal(t, ptr.To("service"), parameters.ServiceName)
			require.Equal(t, ptr.To("standard"), parameters.PlanName)
			require.Equal(t, ptr.To("plan-id"), parameters.PlanUniqueIdentifier)
			require.Equal(t, ptr.To(true), parameters.AutoAssign)
			require.Nil(t, parameters.Distribute)
			if tt.numeric {
				require.Equal(t, ptr.To(0.0), parameters.Amount)
				require.Equal(t, ptr.To(2.0), parameters.AutoDistributeAmount)
			} else {
				require.Nil(t, parameters.Amount)
				require.Nil(t, parameters.AutoDistributeAmount)
			}
			_, hasComment := result.Comment()
			require.False(t, hasComment)
		})
	}
}

func TestConvertDirectoryEntitlementMissingOptionalFields(t *testing.T) {
	t.Parallel()
	e := &directoryEntitlement{
		directoryID: "dir-one", serviceName: "service",
		plan:                btpcli.DirectoryEntitledServicePlan{Name: "standard", Category: "SERVICE"},
		ResourceWithComment: yaml.NewResourceWithComment(nil),
	}
	result := convertDirectoryEntitlementResource(e)
	cr := result.Resource().(*v1alpha1.DirectoryEntitlement)
	require.Equal(t, ptr.To("dir-one"), cr.Spec.ForProvider.DirectoryID)
	require.Nil(t, cr.Spec.ForProvider.DirectoryRef)
	require.Nil(t, cr.Spec.ForProvider.Amount)
	require.Nil(t, cr.Spec.ForProvider.AutoDistributeAmount)
	require.Nil(t, cr.Spec.ForProvider.PlanUniqueIdentifier)
	require.Equal(t, ptr.To(false), cr.Spec.ForProvider.AutoAssign)
	require.Nil(t, cr.Spec.ForProvider.Distribute)
	_, hasComment := result.Comment()
	require.False(t, hasComment)
}

func TestConvertDirectoryEntitlementMissingIdentity(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"directory", "service", "plan"} {
		t.Run(field, func(t *testing.T) {
			e := &directoryEntitlement{
				directoryID: "dir-one", serviceName: "service",
				plan:                btpcli.DirectoryEntitledServicePlan{Name: "standard"},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			}
			switch field {
			case "directory":
				e.directoryID = ""
			case "service":
				e.serviceName = ""
			case "plan":
				e.plan.Name = ""
			}
			result := convertDirectoryEntitlementResource(e)
			cr := result.Resource().(*v1alpha1.DirectoryEntitlement)
			require.Equal(t, resources.UndefinedName, cr.Name)
			require.Equal(t, resources.UndefinedExternalName, cr.Annotations["crossplane.io/external-name"])
			comment, _ := result.Comment()
			require.Contains(t, comment, resources.WarnUndefinedExternalName)
		})
	}
}
