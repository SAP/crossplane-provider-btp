package subscription

import (
	"encoding/json"
	"testing"

	"github.com/SAP/xp-clifford/yaml"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/stretchr/testify/require"
)

func TestConvertSubscriptionResource(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name         string
		app          string
		plan         string
		cm           string
		externalName string
		resourceName string
		warning      string
	}{
		{name: "standard plan", app: "my-app", plan: "standard", cm: "cloud-management", externalName: "my-app/standard", resourceName: "my-app.standard.sa-id"},
		{name: "empty plan", app: "my-app", cm: "cloud-management", externalName: "my-app/", resourceName: "my-app.sa-id"},
		{name: "missing app", plan: "standard", cm: "cloud-management", externalName: resources.UndefinedExternalName, resourceName: resources.UndefinedName, warning: resources.WarnUndefinedExternalName},
		{name: "missing cloud management", app: "my-app", plan: "standard", externalName: "my-app/standard", resourceName: "my-app.standard.sa-id", warning: resources.WarnMissingCloudManagementName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &Subscription{
				Subscription: &btpcli.Subscription{AppName: tt.app, PlanName: tt.plan},
				SubaccountID: "sa-id", CloudManagementName: tt.cm,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			}
			result := convertSubscriptionResource(s)
			cr := result.Resource().(*v1alpha1.Subscription)
			require.Equal(t, "Subscription", cr.Kind)
			require.Equal(t, "account.btp.sap.crossplane.io/v1alpha1", cr.APIVersion)
			require.Equal(t, tt.externalName, cr.Annotations["crossplane.io/external-name"])
			require.Equal(t, tt.resourceName, cr.Name)
			require.Equal(t, tt.app, cr.Spec.ForProvider.AppName)
			require.Equal(t, tt.plan, cr.Spec.ForProvider.PlanName)
			require.Equal(t, xpv1.ManagementPolicies{xpv1.ManagementActionObserve}, cr.Spec.ManagementPolicies)
			require.Equal(t, &xpv1.Reference{Name: tt.cm}, cr.Spec.CloudManagementRef)
			data, err := json.Marshal(cr.Spec)
			require.NoError(t, err)
			require.NotContains(t, string(data), "subaccount")
			require.NotContains(t, string(data), "cloudManagementSecret")
			comment, hasComment := result.Comment()
			if tt.warning != "" {
				require.Contains(t, comment, tt.warning)
			} else {
				require.False(t, hasComment)
			}
		})
	}
}

func TestConvertSubscriptionResource_Parameters(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		parameters json.RawMessage
	}{
		{name: "no parameters"},
		{name: "nested values and large integer", parameters: json.RawMessage(`{"nested":{"enabled":true},"largeNumber":9007199254740993}`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &Subscription{
				Subscription:        &btpcli.Subscription{AppName: "my-app", PlanName: "standard", SubscriptionGUID: "guid-one"},
				SubaccountID:        "sa-one",
				CloudManagementName: "cloud-management",
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				Parameters:          tt.parameters,
			}
			cr := convertSubscriptionResource(s).Resource().(*v1alpha1.Subscription)
			require.Equal(t, "my-app/standard", cr.Annotations["crossplane.io/external-name"])
			if tt.parameters == nil {
				require.Empty(t, cr.Spec.ForProvider.SubscriptionParameters.Raw)
				return
			}
			require.JSONEq(t, string(tt.parameters), string(cr.Spec.ForProvider.SubscriptionParameters.Raw))
			data, err := json.Marshal(cr.Spec.ForProvider)
			require.NoError(t, err)
			require.Contains(t, string(data), `"largeNumber":9007199254740993`)
		})
	}
}
