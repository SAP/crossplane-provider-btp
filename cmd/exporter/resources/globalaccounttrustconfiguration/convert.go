package globalaccounttrustconfiguration

import (
	"fmt"
	"strconv"

	"github.com/SAP/xp-clifford/yaml"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func convertTrustConfigurationResource(trust *trustConfiguration) *yaml.ResourceWithComment {
	parameters := v1alpha1.GlobalaccountTrustConfigurationParameters{
		Origin:           optionalString(trust.OriginKey),
		Name:             optionalString(trust.Name),
		Description:      optionalString(trust.Description),
		IdentityProvider: optionalString(trust.IdentityProvider),
		Domain:           optionalString(trust.Domain),
	}
	cr := &v1alpha1.GlobalaccountTrustConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.GlobalaccountTrustConfiguration_Kind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        trust.GenerateK8sResourceName(),
			Annotations: map[string]string{"crossplane.io/external-name": trust.GetExternalName()},
		},
		Spec: v1alpha1.GlobalaccountTrustConfigurationSpec{
			ClusterManagedResourceSpec: xpv1.ClusterManagedResourceSpec{
				ManagementPolicies: []xpv1.ManagementAction{xpv1.ManagementActionObserve},
			},
			ForProvider: parameters,
		},
	}
	result := yaml.NewResourceWithComment(cr)
	result.CloneComment(trust)
	// This flag is a string boolean in the CLI response. Never silently map
	// a missing or malformed value to false.
	cr.Spec.ForProvider.AutoCreateShadowUsers = parseBoolean(trust.CreateShadowUsersDuringLogon, "createShadowUsersDuringLogon", result)
	if trust.OriginKey == "" {
		result.AddComment(resources.WarnMissingTrustOrigin)
	}
	if parameters.IdentityProvider == nil {
		result.AddComment(resources.WarnMissingIdentityProvider)
	}
	if cr.Name == resources.UndefinedName {
		result.AddComment(resources.WarnUndefinedResourceName)
	}
	if trust.GetExternalName() == resources.UndefinedExternalName {
		result.AddComment(resources.WarnUndefinedExternalName)
	}
	return result
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return ptr.To(value)
}

func parseBoolean(value, field string, result *yaml.ResourceWithComment) *bool {
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		result.AddComment(fmt.Sprintf("WARNING: invalid %s boolean: %q", field, value))
		return nil
	}
	return ptr.To(parsed)
}
