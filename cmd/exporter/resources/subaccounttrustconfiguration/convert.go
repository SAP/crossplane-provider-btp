package subaccounttrustconfiguration

import (
	"context"
	"fmt"
	"strconv"

	"github.com/SAP/xp-clifford/cli/export"
	"github.com/SAP/xp-clifford/yaml"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func convertTrustConfigurationResource(ctx context.Context, client *btpcli.BtpCli, trust *trustConfiguration, handler export.EventHandler, resolveReferences bool) *yaml.ResourceWithComment {
	parameters := v1alpha1.SubaccountTrustConfigurationParameters{
		SubaccountID:     ptr.To(trust.SubaccountID),
		Origin:           optionalString(trust.OriginKey),
		Name:             optionalString(trust.Name),
		Description:      optionalString(trust.Description),
		IdentityProvider: optionalString(trust.IdentityProvider),
		Domain:           optionalString(trust.Domain),
		LinkText:         optionalString(trust.LinkTextForUserLogon),
		Status:           optionalString(trust.Status),
	}
	// The built-in trust has no IAS tenant.
	if trust.OriginKey == "sap.default" && trust.IdentityProvider == "" {
		parameters.IdentityProvider = ptr.To("")
	}
	cr := &v1alpha1.SubaccountTrustConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.SubaccountTrustConfiguration_Kind,
			APIVersion: v1alpha1.CRDGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        trust.GenerateK8sResourceName(),
			Annotations: map[string]string{"crossplane.io/external-name": trust.GetExternalName()},
		},
		Spec: v1alpha1.SubaccountTrustConfigurationSpec{
			ClusterManagedResourceSpec: xpv1.ClusterManagedResourceSpec{
				ManagementPolicies: []xpv1.ManagementAction{xpv1.ManagementActionObserve},
			},
			ForProvider: parameters,
		},
	}
	result := yaml.NewResourceWithComment(cr)
	result.CloneComment(trust)
	// These flags are string booleans in the CLI response. Never silently map
	// a missing or malformed value to false.
	cr.Spec.ForProvider.AvailableForUserLogon = parseBoolean(trust.AvailableForUserLogon, "availableForUserLogon", result)
	cr.Spec.ForProvider.AutoCreateShadowUsers = parseBoolean(trust.CreateShadowUsersDuringLogon, "createShadowUsersDuringLogon", result)
	if trust.SubaccountID == "" {
		result.AddComment(resources.WarnMissingSubaccountGuid)
	}
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
	if resolveReferences {
		name, err := subaccount.GetK8sResourceNameByID(ctx, client, trust.SubaccountID)
		if err != nil {
			handler.Warn(fmt.Errorf("cannot resolve subaccount reference for trust %s: %w", trust.GetID(), err))
			result.AddComment(resources.WarnCannotResolveSubaccount + ": " + trust.SubaccountID)
		} else if name == resources.UndefinedName {
			handler.Warn(fmt.Errorf("cannot resolve subaccount reference for trust %s: resource name is undefined", trust.GetID()))
			result.AddComment(resources.WarnCannotResolveSubaccount + ": " + trust.SubaccountID)
		} else {
			cr.Spec.ForProvider.SubaccountRef = &xpv1.Reference{Name: name}
			cr.Spec.ForProvider.SubaccountID = nil
		}
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
