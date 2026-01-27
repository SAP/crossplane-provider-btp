package resources

import (
	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/parsan"
)

const UNDEFINED_NAME = "UNDEFINED-NAME"

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
