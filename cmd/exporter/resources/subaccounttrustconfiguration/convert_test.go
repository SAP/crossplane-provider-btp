package subaccounttrustconfiguration

import (
	"encoding/json"
	"testing"

	"github.com/SAP/xp-clifford/yaml"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestConvertTrustConfigurationResource(t *testing.T) {
	t.Parallel()
	trust := &trustConfiguration{
		SubaccountID: "sa-one",
		TrustConfiguration: &btpcli.TrustConfiguration{
			OriginKey: "sap.custom", Name: "Custom trust", Description: "Test description",
			IdentityProvider: "tenant.example.com", Domain: "login.example.com",
			LinkTextForUserLogon: "Sign in", Status: "inactive",
			AvailableForUserLogon: "false", CreateShadowUsersDuringLogon: "true",
		},
		ResourceWithComment: yaml.NewResourceWithComment(nil),
	}
	result := convertTrustConfigurationResource(t.Context(), nil, trust, &recordingHandler{}, false)
	cr := result.Resource().(*v1alpha1.SubaccountTrustConfiguration)
	require.Equal(t, "SubaccountTrustConfiguration", cr.Kind)
	require.Equal(t, "security.btp.sap.crossplane.io/v1alpha1", cr.APIVersion)
	require.Equal(t, "sap.custom.sa-one", cr.Name)
	require.Equal(t, "sa-one/sap.custom", cr.Annotations["crossplane.io/external-name"])
	require.Equal(t, xpv1.ManagementPolicies{xpv1.ManagementActionObserve}, cr.Spec.ManagementPolicies)
	require.Equal(t, v1alpha1.SubaccountTrustConfigurationParameters{
		SubaccountID: ptr.To("sa-one"), Origin: ptr.To("sap.custom"), Name: ptr.To("Custom trust"),
		Description: ptr.To("Test description"), IdentityProvider: ptr.To("tenant.example.com"),
		Domain: ptr.To("login.example.com"), LinkText: ptr.To("Sign in"), Status: ptr.To("inactive"),
		AvailableForUserLogon: ptr.To(false), AutoCreateShadowUsers: ptr.To(true),
	}, cr.Spec.ForProvider)
	_, hasComment := result.Comment()
	require.False(t, hasComment)
	data, err := json.Marshal(cr.Spec.ForProvider)
	require.NoError(t, err)
	require.Contains(t, string(data), `"availableForUserLogon":false`)
}

func TestConvertDefaultTrust(t *testing.T) {
	t.Parallel()
	trust := &trustConfiguration{
		SubaccountID: "sa-one",
		TrustConfiguration: &btpcli.TrustConfiguration{
			OriginKey: "sap.default", Name: "sap.default",
			AvailableForUserLogon: "true", CreateShadowUsersDuringLogon: "false",
		},
		ResourceWithComment: yaml.NewResourceWithComment(nil),
	}
	result := convertTrustConfigurationResource(t.Context(), nil, trust, &recordingHandler{}, false)
	cr := result.Resource().(*v1alpha1.SubaccountTrustConfiguration)
	require.Equal(t, "sa-one/sap.default", cr.Annotations["crossplane.io/external-name"])
	require.Equal(t, ptr.To(""), cr.Spec.ForProvider.IdentityProvider)
	require.Nil(t, cr.Spec.ForProvider.Domain)
	require.Nil(t, cr.Spec.ForProvider.Description)
	require.Equal(t, ptr.To(false), cr.Spec.ForProvider.AutoCreateShadowUsers)
	_, hasComment := result.Comment()
	require.False(t, hasComment)
}

func TestTrustConfigurationWarnings(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name             string
		subaccountID     string
		origin           string
		identityProvider string
		warning          string
	}{
		{name: "missing subaccount", origin: "custom", identityProvider: "tenant.example.com", warning: resources.WarnMissingSubaccountGuid},
		{name: "missing origin", subaccountID: "sa-one", identityProvider: "tenant.example.com", warning: resources.WarnMissingTrustOrigin},
		{name: "missing IAS tenant", subaccountID: "sa-one", origin: "custom", warning: resources.WarnMissingIdentityProvider},
	} {
		t.Run(tt.name, func(t *testing.T) {
			trust := &trustConfiguration{
				SubaccountID:        tt.subaccountID,
				TrustConfiguration:  &btpcli.TrustConfiguration{OriginKey: tt.origin, IdentityProvider: tt.identityProvider},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			}
			result := convertTrustConfigurationResource(t.Context(), nil, trust, &recordingHandler{}, false)
			comment, _ := result.Comment()
			require.Contains(t, comment, tt.warning)
		})
	}
}

func TestParseBoolean(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		value string
		want  *bool
		warn  bool
	}{
		{value: "true", want: ptr.To(true)},
		{value: "false", want: ptr.To(false)},
		{value: ""},
		{value: "invalid", warn: true},
	} {
		t.Run(tt.value, func(t *testing.T) {
			result := yaml.NewResourceWithComment(nil)
			require.Equal(t, tt.want, parseBoolean(tt.value, "availableForUserLogon", result))
			_, hasComment := result.Comment()
			require.Equal(t, tt.warn, hasComment)
		})
	}
}

func TestTrustReferenceFailureRetainsSubaccountID(t *testing.T) {
	t.Parallel()
	trust := &trustConfiguration{
		TrustConfiguration:  &btpcli.TrustConfiguration{OriginKey: "custom", IdentityProvider: "tenant.example.com"},
		ResourceWithComment: yaml.NewResourceWithComment(nil),
	}
	handler := &recordingHandler{}
	result := convertTrustConfigurationResource(t.Context(), nil, trust, handler, true)
	cr := result.Resource().(*v1alpha1.SubaccountTrustConfiguration)
	require.Equal(t, ptr.To(""), cr.Spec.ForProvider.SubaccountID)
	require.Nil(t, cr.Spec.ForProvider.SubaccountRef)
	require.Nil(t, cr.Spec.ForProvider.AvailableForUserLogon)
	require.Nil(t, cr.Spec.ForProvider.AutoCreateShadowUsers)
	require.Len(t, handler.warnings, 1)
	require.ErrorContains(t, handler.warnings[0], "cannot resolve subaccount reference")
}
