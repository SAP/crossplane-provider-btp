package testutils

import (
	"reflect"
	"unsafe"

	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

// NewAccountAPIError builds a *accountclient.GenericOpenAPIError carrying an
// ApiExceptionResponseObject with the given code/message and a transport-level
// error string. Mimics what the BTP accounts SDK returns for non-2xx responses
// so tests can exercise specifyAPIError-style unwrapping.
//
// The unexported `model` and `error` fields are set via unsafe-pointer
// reflection because the SDK exposes no constructor; if the SDK is regenerated
// and renames either field this helper will silently no-op — fix it here, in
// one place, instead of N copies across packages.
func NewAccountAPIError(code float32, message, transportErr string) error {
	apiExceptionError := accountclient.NewApiExceptionResponseObjectError()
	apiExceptionError.SetCode(code)
	if message != "" {
		apiExceptionError.SetMessage(message)
	}
	apiException := accountclient.NewApiExceptionResponseObject(*apiExceptionError)

	err := &accountclient.GenericOpenAPIError{}
	errValue := reflect.ValueOf(err).Elem()

	if modelField := errValue.FieldByName("model"); modelField.IsValid() {
		reflect.NewAt(modelField.Type(), unsafe.Pointer(modelField.UnsafeAddr())).
			Elem().Set(reflect.ValueOf(*apiException))
	}
	if errorField := errValue.FieldByName("error"); errorField.IsValid() {
		reflect.NewAt(errorField.Type(), unsafe.Pointer(errorField.UnsafeAddr())).
			Elem().SetString(transportErr)
	}
	return err
}
