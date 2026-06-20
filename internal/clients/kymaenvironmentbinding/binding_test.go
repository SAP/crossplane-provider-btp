package kymaenvironmentbinding

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/clients/fakes"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

// bindingMock embeds the package-level mock and overrides the two endpoints
// exercised by DescribeInstance and DeleteInstances so we can drive specific
// error/response pairs.
type bindingMock struct {
	fakes.MockProvisioningServiceClient

	getAllErr error

	deleteResp *http.Response
	deleteErr  error
}

func (m *bindingMock) GetAllEnvironmentInstanceBindings(_ context.Context, _ string) provisioningclient.ApiGetAllEnvironmentInstanceBindingsRequest {
	return provisioningclient.ApiGetAllEnvironmentInstanceBindingsRequest{ApiService: m}
}

func (m *bindingMock) GetAllEnvironmentInstanceBindingsExecute(_ provisioningclient.ApiGetAllEnvironmentInstanceBindingsRequest) (*provisioningclient.GetAllInstanceBindingsResponse, *http.Response, error) {
	return nil, &http.Response{}, m.getAllErr
}

func (m *bindingMock) DeleteEnvironmentInstanceBinding(_ context.Context, _ string, _ string) provisioningclient.ApiDeleteEnvironmentInstanceBindingRequest {
	return provisioningclient.ApiDeleteEnvironmentInstanceBindingRequest{ApiService: m}
}

func (m *bindingMock) DeleteEnvironmentInstanceBindingExecute(_ provisioningclient.ApiDeleteEnvironmentInstanceBindingRequest) (*provisioningclient.DeleteEnvironmentInstanceBindingResponse, *http.Response, error) {
	return nil, m.deleteResp, m.deleteErr
}

func TestDescribeInstance_APIErrorWrappedWithSpecifyAPIError(t *testing.T) {
	mock := &bindingMock{getAllErr: newProvisioningAPIError(500, "internal server error")}
	c := KymaBindings{btp: btp.Client{ProvisioningServiceClient: mock}}

	_, err := c.DescribeInstance(context.Background(), "kyma-id")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), errKymaBindingDescribeFailed) {
		t.Errorf("expected wrap %q, got %q", errKymaBindingDescribeFailed, err.Error())
	}
	if !strings.Contains(err.Error(), "API Error") {
		t.Errorf("expected specifyAPIError to surface 'API Error' prefix, got %q", err.Error())
	}
}

func TestDeleteInstances_APIErrorWrappedWithSpecifyAPIError(t *testing.T) {
	mock := &bindingMock{
		deleteResp: &http.Response{StatusCode: 500, Status: "500 Internal Server Error"},
		deleteErr:  newProvisioningAPIError(500, "internal server error"),
	}
	c := KymaBindings{btp: btp.Client{ProvisioningServiceClient: mock}}

	err := c.DeleteInstances(context.Background(), []v1alpha1.Binding{{Id: "binding-id"}}, "kyma-id")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), errKymaBindingDeleteFailed) {
		t.Errorf("expected wrap %q, got %q", errKymaBindingDeleteFailed, err.Error())
	}
	if !strings.Contains(err.Error(), "API Error") {
		t.Errorf("expected specifyAPIError to surface 'API Error' prefix, got %q", err.Error())
	}
}

func TestDeleteInstances_404IsSwallowed(t *testing.T) {
	mock := &bindingMock{
		deleteResp: &http.Response{StatusCode: 404},
		deleteErr:  newProvisioningAPIError(404, "not found"),
	}
	c := KymaBindings{btp: btp.Client{ProvisioningServiceClient: mock}}

	if err := c.DeleteInstances(context.Background(), []v1alpha1.Binding{{Id: "binding-id"}}, "kyma-id"); err != nil {
		t.Fatalf("expected nil error on 404, got %v", err)
	}
}

// newProvisioningAPIError builds a provisioningclient.GenericOpenAPIError with
// an embedded ApiExceptionResponseObject so that specifyAPIError surfaces an
// "API Error: ..." prefix. Mirrors the create500Error helper in subaccount
// tests; reflect+unsafe is required because both struct fields are unexported.
func newProvisioningAPIError(code int32, message string) error {
	apiErr := provisioningclient.NewApiExceptionResponseObjectError(code, "corr-id", message)
	model := provisioningclient.ApiExceptionResponseObject{Error: apiErr}

	genericErr := &provisioningclient.GenericOpenAPIError{}
	v := reflect.ValueOf(genericErr).Elem()

	if f := v.FieldByName("model"); f.IsValid() {
		reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.ValueOf(model))
	}
	if f := v.FieldByName("error"); f.IsValid() {
		reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().SetString("api error")
	}
	return genericErr
}
