package testutils

import (
	"encoding/json"
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

// NewAccountAPIErrorFromBody builds a *accountclient.GenericOpenAPIError from a
// raw accounts-service response body, decoding it into the model the same way
// the SDK does and keeping the body alongside it.
//
// Use this rather than NewAccountAPIError when a test needs parts of the
// response the typed constructor cannot express — notably the "details" array,
// which the generated model has no field for and which only survives in
// ApiExceptionResponseObjectError.AdditionalProperties after unmarshaling.
//
// Panics on a body that is not a valid ApiExceptionResponseObject, so a typo in
// a test fixture fails loudly instead of silently producing an empty model.
func NewAccountAPIErrorFromBody(body []byte, transportErr string) error {
	var apiException accountclient.ApiExceptionResponseObject
	if err := json.Unmarshal(body, &apiException); err != nil {
		panic("testutils: invalid ApiExceptionResponseObject body: " + err.Error())
	}

	err := &accountclient.GenericOpenAPIError{}
	setUnexportedAccountErrField(err, "model", reflect.ValueOf(apiException))
	setUnexportedAccountErrField(err, "body", reflect.ValueOf(body))
	setUnexportedAccountErrField(err, "error", reflect.ValueOf(transportErr))
	return err
}

// NewAccountAPIErrorBodyOnly builds a *accountclient.GenericOpenAPIError that
// carries a response body but no model, as the SDK does when it cannot decode
// the payload into a known type.
func NewAccountAPIErrorBodyOnly(body []byte, transportErr string) error {
	err := &accountclient.GenericOpenAPIError{}
	setUnexportedAccountErrField(err, "body", reflect.ValueOf(body))
	setUnexportedAccountErrField(err, "error", reflect.ValueOf(transportErr))
	return err
}

// setUnexportedAccountErrField writes one of GenericOpenAPIError's unexported
// fields. Same unsafe-pointer approach and same caveat as NewAccountAPIError: a
// regenerated SDK that renames a field makes this a no-op, and this is the one
// place to fix it.
func setUnexportedAccountErrField(err *accountclient.GenericOpenAPIError, name string, value reflect.Value) {
	field := reflect.ValueOf(err).Elem().FieldByName(name)
	if !field.IsValid() {
		return
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(value)
}
