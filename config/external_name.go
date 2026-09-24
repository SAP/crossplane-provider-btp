/*
Copyright 2022 Upbound Inc.
*/

package config

import "github.com/crossplane/upjet/v2/pkg/config"

// TerraformPluginFrameworkReconciledExternalNameConfigs contains all external name configurations for this
// provider that use the terraform plugin framework external client.
var TerraformPluginFrameworkReconciledExternalNameConfigs = map[string]config.ExternalName{
	"btp_subaccount_trust_configuration":    config.IdentifierFromProvider,
	"btp_globalaccount_trust_configuration": config.IdentifierFromProvider,
	"btp_directory_entitlement":             config.IdentifierFromProvider,
	"btp_subaccount_service_broker":         config.IdentifierFromProvider,
	"btp_subaccount_api_credential":         config.IdentifierFromProvider,
	"btp_subaccount_service_instance":       config.IdentifierFromProvider,
	"btp_subaccount_service_binding":        config.IdentifierFromProvider,
}

// ExternalNameConfigurations applies all external name configs listed in
// TerraformPluginFrameworkReconciledExternalNameConfigs and sets the version of
// those resources to v1beta1 assuming they will be tested.
func ExternalNameConfigurations() config.ResourceOption {
	return func(r *config.Resource) {
		if externalName, ok := TerraformPluginFrameworkReconciledExternalNameConfigs[r.Name]; ok {
			r.ExternalName = externalName
		}
	}
}

// TerraformPluginFrameworkReconciledResourceList returns the list of all resources whose external name
// is configured manually and reconciled via the Terraform Plugin Framework external client.
func TerraformPluginFrameworkReconciledResourceList() []string {
	return externalNameResourceList(TerraformPluginFrameworkReconciledExternalNameConfigs)
}

// externalNameResourceList builds the regex-anchored resource name list that
// upjet's include-list options expect from a map of external-name configs.
func externalNameResourceList(m map[string]config.ExternalName) []string {
	l := make([]string, 0, len(m))
	for name := range m {
		// $ is added to match the exact string since the format is regex.
		l = append(l, name+"$")
	}
	return l
}
