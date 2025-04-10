/*
Copyright 2021 Upbound Inc.
*/

package config

import (
	// Note(turkenh): we are importing this to embed provider schema document
	_ "embed"

	ujconfig "github.com/crossplane/upjet/pkg/config"
	apicredentials "github.com/sap/crossplane-provider-btp/config/btp_subaccount_api_credential"
	directoryentitlement "github.com/sap/crossplane-provider-btp/config/directory_entitlement"
	globaltrustconfig "github.com/sap/crossplane-provider-btp/config/globalaccount_trust_configuration"
	servicebinding "github.com/sap/crossplane-provider-btp/config/subaccount_service_binding"
	servicebroker "github.com/sap/crossplane-provider-btp/config/subaccount_service_broker"
	serviceinstance "github.com/sap/crossplane-provider-btp/config/subaccount_service_instance"
	trustconfig "github.com/sap/crossplane-provider-btp/config/subaccount_trust_configuration"
)

const (
	resourcePrefix = "account"
	modulePath     = "github.com/sap/crossplane-provider-btp"
)

//go:embed schema.json
var providerSchema string

//go:embed provider-metadata.yaml
var providerMetadata string

// GetProvider returns provider configuration
func GetProvider() *ujconfig.Provider {
	pc := ujconfig.NewProvider([]byte(providerSchema), resourcePrefix, modulePath, []byte(providerMetadata),
		ujconfig.WithRootGroup("btp.sap.crossplane.io"),
		ujconfig.WithIncludeList(ExternalNameConfigured()),
		ujconfig.WithFeaturesPackage("internal/features"),
		ujconfig.WithDefaultResourceOptions(
			ExternalNameConfigurations(),
		))

	for _, configure := range []func(provider *ujconfig.Provider){
		// add custom config functions
		trustconfig.Configure,
		globaltrustconfig.Configure,
		directoryentitlement.Configure,
		serviceinstance.Configure,
		servicebinding.Configure,
		servicebroker.Configure,
		apicredentials.Configure,
	} {
		configure(pc)
	}

	pc.ConfigureResources()
	return pc
}
