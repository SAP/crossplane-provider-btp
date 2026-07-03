package testutils

import (
	"reflect"
	"unsafe"

	xsuaa "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-xsuaa-service-api-go/pkg"
)

// NewXsuaaAPIError builds a *xsuaa.GenericOpenAPIError whose unexported `body`
// field carries the given response body. Used in tests that exercise the
// SpecifyAPIError unwrapping path. If the XSUAA SDK is regenerated and renames
// the field this helper will silently no-op — fix it here, in one place,
// instead of N copies across packages.
//
// Lives in its own file (not openapi_errors.go) so it does not collide on
// rebase with the account-side helper introduced by PR #731.
func NewXsuaaAPIError(body []byte) error {
	e := &xsuaa.GenericOpenAPIError{}
	if body == nil {
		return e
	}
	f := reflect.ValueOf(e).Elem().FieldByName("body")
	if !f.IsValid() {
		return e
	}
	reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.ValueOf(body))
	return e
}
