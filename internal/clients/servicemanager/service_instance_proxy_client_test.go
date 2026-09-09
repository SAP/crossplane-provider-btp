package servicemanager

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/pkg/errors"
	saops "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

func response(code int) *http.Response {
	return &http.Response{StatusCode: code}
}

// TestCreateAdminBindingSurfacesAPIBody asserts that createAdminBinding routes
// transport errors through specifyAccountsAPIError so the BTP API body is surfaced
// instead of the opaque "<status> <reason>" string. Driven via EnsureSemanticLookuper,
// which mints an admin binding when none exists (404 -> create).
func TestCreateAdminBindingSurfacesAPIBody(t *testing.T) {
	accountService := &SubaccountServiceFake{
		GetMockFn: func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
			return nil, response(404), errors.New("not found")
		},
		CreateMockFn: func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
			return nil, response(500), create500Error()
		},
	}
	smClient := ServiceManagerInstanceProxyClient{accountService}

	_, _, err := smClient.EnsureSemanticLookuper(context.TODO(), "")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "internal server error") || !strings.Contains(err.Error(), "Code 500") {
		t.Errorf("expected SM API body to be surfaced, got: %v", err)
	}
}

// create500Error builds a *saops.GenericOpenAPIError carrying an
// ApiExceptionResponseObject so we can assert specifyAccountsAPIError surfaces the
// structured body. Mirrors subaccount_test.go's helper.
func create500Error() error {
	apiExceptionError := saops.NewApiExceptionResponseObjectError()
	apiExceptionError.SetCode(500)
	apiExceptionError.SetMessage("internal server error")

	apiException := saops.NewApiExceptionResponseObject(*apiExceptionError)

	err := &saops.GenericOpenAPIError{}
	errValue := reflect.ValueOf(err).Elem()

	if modelField := errValue.FieldByName("model"); modelField.IsValid() {
		reflect.NewAt(modelField.Type(), unsafe.Pointer(modelField.UnsafeAddr())).
			Elem().Set(reflect.ValueOf(*apiException))
	}
	if errorField := errValue.FieldByName("error"); errorField.IsValid() {
		reflect.NewAt(errorField.Type(), unsafe.Pointer(errorField.UnsafeAddr())).
			Elem().SetString("500 Internal Server Error")
	}
	return err
}
