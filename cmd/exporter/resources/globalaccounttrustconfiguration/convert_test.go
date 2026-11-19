package globalaccounttrustconfiguration

import (
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
		TrustConfiguration: &btpcli.TrustConfiguration{
			OriginKey: "sap.custom", Name: "Custom trust", Description: "Test description",
			IdentityProvider: "tenant.example.com", Domain: "login.example.com",
			Status: "inactive", CreateShadowUsersDuringLogon: "true",
		},
		ResourceWithComment: yaml.NewResourceWithComment(nil),
	}
	result := convertTrustConfigurationResource(trust)
	cr := result.Resource().(*v1alpha1.GlobalaccountTrustConfiguration)
	require.Equal(t, "GlobalaccountTrustConfiguration", cr.Kind)
	require.Equal(t, "security.btp.sap.crossplane.io/v1alpha1", cr.APIVersion)
	require.Equal(t, "sap.custom", cr.Name)
	require.Equal(t, "sap.custom", cr.Annotations["crossplane.io/external-name"])
	require.Equal(t, xpv1.ManagementPolicies{xpv1.ManagementActionObserve}, cr.Spec.ManagementPolicies)
	require.Equal(t, v1alpha1.GlobalaccountTrustConfigurationParameters{
		Origin: ptr.To("sap.custom"), Name: ptr.To("Custom trust"),
		Description: ptr.To("Test description"), IdentityProvider: ptr.To("tenant.example.com"),
		Domain: ptr.To("login.example.com"), AutoCreateShadowUsers: ptr.To(true),
	}, cr.Spec.ForProvider)
	_, hasComment := result.Comment()
	require.False(t, hasComment)
}

func TestTrustConfigurationWarnings(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name             string
		origin           string
		identityProvider string
		warning          string
	}{
		{name: "missing origin", identityProvider: "tenant.example.com", warning: resources.WarnMissingTrustOrigin},
		{name: "missing IAS tenant", origin: "custom", warning: resources.WarnMissingIdentityProvider},
	} {
		t.Run(tt.name, func(t *testing.T) {
			trust := &trustConfiguration{
				TrustConfiguration:  &btpcli.TrustConfiguration{OriginKey: tt.origin, IdentityProvider: tt.identityProvider},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			}
			result := convertTrustConfigurationResource(trust)
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
			require.Equal(t, tt.want, parseBoolean(tt.value, "createShadowUsersDuringLogon", result))
			_, hasComment := result.Comment()
			require.Equal(t, tt.warn, hasComment)
		})
	}
}
