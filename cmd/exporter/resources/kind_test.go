package resources_test

import (
	"testing"

	"github.com/SAP/xp-clifford/cli/configparam"
	"github.com/stretchr/testify/require"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/cfenvironment"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/entitlement"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/servicebinding"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/serviceinstance"
	_ "github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
)

func TestKindNamesAreSortedAndIncludeEverySupportedKind(t *testing.T) {
	kindNames := resources.KindNames()

	require.Equal(t, []string{
		"cloudfoundry-environment",
		"entitlement",
		"servicebinding",
		"serviceinstance",
		"subaccount",
	}, kindNames)
}

func TestConfigParamsIncludesEveryKindSelector(t *testing.T) {
	params := resources.ConfigParams()
	paramNames := make([]string, 0, len(params))

	for _, param := range params {
		_, isStringSlice := param.(*configparam.StringSliceParam)
		require.Truef(t, isStringSlice, "kind selector %q must be a string-slice parameter", param.GetName())
		paramNames = append(paramNames, param.GetName())
	}

	require.Equal(t, resources.KindNames(), paramNames)
}
