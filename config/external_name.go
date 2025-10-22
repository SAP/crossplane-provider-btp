/*
Copyright 2022 Upbound Inc.
*/

package config

import "github.com/crossplane/upjet/pkg/config"

// CLIReconciledExternalNameConfigs contains all external name configurations for this
// provider that use the default (CLI) external client.
var CLIReconciledExternalNameConfigs = map[string]config.ExternalName{
	"btp_subaccount_api_credential": config.IdentifierFromProvider,
}

// TerraformPluginFrameworkReconciledExternalNameConfigs contains all external name configurations for this
// provider that use the terraform plugin framework external client.
var TerraformPluginFrameworkReconciledExternalNameConfigs = map[string]config.ExternalName{
	"btp_subaccount_trust_configuration":    config.IdentifierFromProvider,
	"btp_globalaccount_trust_configuration": config.IdentifierFromProvider,
	"btp_directory_entitlement":             config.IdentifierFromProvider,
	"btp_subaccount_service_instance":       config.IdentifierFromProvider,
	"btp_subaccount_service_binding":        config.IdentifierFromProvider,
	"btp_subaccount_service_broker":         config.IdentifierFromProvider,
}

// ExternalNameConfigurations applies all external name configs listed in the
// table ExternalNameConfigs and sets the version of those resources to v1beta1
// assuming they will be tested.
func ExternalNameConfigurations() config.ResourceOption {
	return func(r *config.Resource) {
		// if an external name is configured for multiple architectures,
		// Terraform Plugin Framework is preferred over CLI architecture
		e, configured := TerraformPluginFrameworkReconciledExternalNameConfigs[r.Name]
		if !configured {
			e, configured = CLIReconciledExternalNameConfigs[r.Name]
		}
		if !configured {
			return
		}
		r.ExternalName = e
	}
}

// CLIReconciledResourceList returns the list of all resources whose external name
// is configured manually.
func CLIReconciledResourceList() []string {
	l := make([]string, len(CLIReconciledExternalNameConfigs))
	i := 0
	for name := range CLIReconciledExternalNameConfigs {
		// $ is added to match the exact string since the format is regex.
		l[i] = name + "$"
		i++
	}
	return l
}

// TerraformPluginFrameworkReconciledResourceList returns the list of all resources whose external name
// is configured manually.
func TerraformPluginFrameworkReconciledResourceList() []string {
	l := make([]string, len(TerraformPluginFrameworkReconciledExternalNameConfigs))
	i := 0
	for name := range TerraformPluginFrameworkReconciledExternalNameConfigs {
		// $ is added to match the exact string since the format is regex.
		l[i] = name + "$"
		i++
	}
	return l
}
