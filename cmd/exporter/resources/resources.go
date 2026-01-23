package resources

import (
	"context"

	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/configparam"
	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/export"
	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/parsan"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/client"
)

const UNDEFINED_NAME = "UNDEFINED-NAME"

// Kind interface must be implemented by each BTP provider custom resource kind.
type Kind interface {
	KindName() string
	// Param method returns the configuration parameters specific
	// to a resource kind.
	Param() configparam.ConfigParam
	// Export method performs the export operation of a resource
	// kind. The method first identifies the resources that are to
	// be exported using the values of the related configuration
	// parameters. Then it collects the resource definitions
	// through BTP Client. Finally, the resources are exported
	// using the eventHandler.
	Export(ctx context.Context, btpClient *client.Client, evHandler export.EventHandler, resolveReferences bool) error
}

var kinds = map[string]Kind{}

// RegisterKind function registers a resource kind.
func RegisterKind(kind Kind) {
	kinds[kind.KindName()] = kind
}

// ConfigParams function returns the configuration parameters of all
// registered resource kinds.
func ConfigParams() []configparam.ConfigParam {
	result := make([]configparam.ConfigParam, 0, len(kinds))
	for _, kind := range kinds {
		if p := kind.Param(); p != nil {
			result = append(result, p)
		}
	}
	return result
}

// ExportFn returns the export function of a given kind.
func ExportFn(kind string) func(context.Context, *client.Client, export.EventHandler, bool) error {
	resource, ok := kinds[kind]
	if !ok || resource == nil {
		return nil
	}
	return resource.Export
}

// StringValueOk returns the string value of a *string and a boolean indicating,
// whether the pointer was not nil and the value was not empty.
// Background: OpenAPI generated code often uses pointers for optional fields.
// To access those fields it provides the methods like GetFieldOk() (value *string, ok bool).
// The ok return parameter indicates whether the field was set (not nil).
// This ok parameter is used as a hint.
func StringValueOk(s *string, hint bool) (string, bool) {
	if !hint || s == nil {
		return "", false
	}
	if len(*s) == 0 {
		return "", false
	}
	return *s, true
}

// BoolValueOk returns the bool value of a *bool and a boolean indicating,
// whether the pointer was not nil.
// Background: OpenAPI generated code often uses pointers for optional fields.
// To access those fields it provides the methods like GetFieldOk() (value *bool, ok bool).
// The ok return parameter indicates whether the field was set.
// This ok parameter is used as a hint.
func BoolValueOk(b *bool, hint bool) (bool, bool) {
	if !hint || b == nil {
		return false, false
	}
	return *b, true
}

// FloatValueOk returns the float32 value of a *float32 and a boolean indicating,
// whether the pointer was not nil.
// Background: OpenAPI generated code often uses pointers for optional fields.
// To access those fields it provides the methods like GetFieldOk() (value *float32, ok bool).
// The ok return parameter indicates whether the field was set.
// This ok parameter is used as a hint.
func FloatValueOk(f *float32, hint bool) (float32, bool) {
	if !hint || f == nil {
		return 0, false
	}
	return *f, true
}

func SanitizeK8sResourceName(s string) string {
	suggestions := parsan.ParseAndSanitize(s, parsan.RFC1035LowerSubdomain)
	if len(suggestions) == 0 {
		return UNDEFINED_NAME
	}

	return suggestions[0]
}
