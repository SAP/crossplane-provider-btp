package globalaccount

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
	trackingtest "github.com/sap/crossplane-provider-btp/internal/tracking/test"
)

// TestObserve_GetAPI5xxWrappedWithSpecifyAPIError asserts that a 5xx
// GenericOpenAPIError from GetGlobalAccount is routed through specifyAPIError
// so the BTP body lands in the resource's Synced condition.
func TestObserve_GetAPI5xxWrappedWithSpecifyAPIError(t *testing.T) {
	e := external{
		btp: btp.Client{
			AccountsServiceClient: &accountclient.APIClient{
				GlobalAccountOperationsAPI: &mockGlobalAccountClient{returnErr: create500Error()},
			},
		},
		tracker: trackingtest.NoOpReferenceResolverTracker{},
	}
	_, err := e.Observe(context.Background(), &apisv1alpha1.GlobalAccount{})
	if err == nil || !strings.Contains(err.Error(), "API Error") {
		t.Errorf("expected error containing %q, got %v", "API Error", err)
	}
}

// create500Error mirrors subaccount_test.go: builds a GenericOpenAPIError that
// wraps an ApiExceptionResponseObject with code 500 to drive specifyAPIError's
// model branch.
func create500Error() error {
	apiExceptionError := accountclient.NewApiExceptionResponseObjectError()
	apiExceptionError.SetCode(500)
	apiExceptionError.SetMessage("internal server error")
	apiException := accountclient.NewApiExceptionResponseObject(*apiExceptionError)

	err := &accountclient.GenericOpenAPIError{}
	errValue := reflect.ValueOf(err).Elem()

	modelField := errValue.FieldByName("model")
	if modelField.IsValid() {
		reflect.NewAt(modelField.Type(), unsafe.Pointer(modelField.UnsafeAddr())).
			Elem().Set(reflect.ValueOf(*apiException))
	}
	errorField := errValue.FieldByName("error")
	if errorField.IsValid() {
		reflect.NewAt(errorField.Type(), unsafe.Pointer(errorField.UnsafeAddr())).
			Elem().SetString("500 Internal Server Error")
	}
	return err
}
