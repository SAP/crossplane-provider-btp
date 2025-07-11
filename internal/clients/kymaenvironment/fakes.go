package environments

import (
	"context"
	"net/http"

	client "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

type MockProvisioningServiceClient struct {
	err         error
	apiResponse *client.BusinessEnvironmentInstancesResponseCollection
}

// CreateEnvironmentInstance implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) CreateEnvironmentInstance(ctx context.Context) client.ApiCreateEnvironmentInstanceRequest {
	panic("unimplemented")
}

// CreateEnvironmentInstanceBinding implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) CreateEnvironmentInstanceBinding(ctx context.Context, environmentInstanceId string) client.ApiCreateEnvironmentInstanceBindingRequest {
	panic("unimplemented")
}

// CreateEnvironmentInstanceBindingExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) CreateEnvironmentInstanceBindingExecute(r client.ApiCreateEnvironmentInstanceBindingRequest) (map[string]interface{}, *http.Response, error) {
	panic("unimplemented")
}

// CreateEnvironmentInstanceExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) CreateEnvironmentInstanceExecute(r client.ApiCreateEnvironmentInstanceRequest) (*client.CreatedEnvironmentInstanceResponseObject, *http.Response, error) {
	panic("unimplemented")
}

// CreateEnvironmentInstanceLabels implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) CreateEnvironmentInstanceLabels(ctx context.Context, environmentInstanceId string) client.ApiCreateEnvironmentInstanceLabelsRequest {
	panic("unimplemented")
}

// CreateEnvironmentInstanceLabelsExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) CreateEnvironmentInstanceLabelsExecute(r client.ApiCreateEnvironmentInstanceLabelsRequest) (*client.LabelsResponseObject, *http.Response, error) {
	panic("unimplemented")
}

// DeleteEnvironmentInstance implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) DeleteEnvironmentInstance(ctx context.Context, environmentInstanceId string) client.ApiDeleteEnvironmentInstanceRequest {
	panic("unimplemented")
}

// DeleteEnvironmentInstanceBinding implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) DeleteEnvironmentInstanceBinding(ctx context.Context, environmentInstanceId string, bindingId string) client.ApiDeleteEnvironmentInstanceBindingRequest {
	panic("unimplemented")
}

// DeleteEnvironmentInstanceBindingExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) DeleteEnvironmentInstanceBindingExecute(r client.ApiDeleteEnvironmentInstanceBindingRequest) (*client.DeleteEnvironmentInstanceBindingResponse, *http.Response, error) {
	panic("unimplemented")
}

// DeleteEnvironmentInstanceExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) DeleteEnvironmentInstanceExecute(r client.ApiDeleteEnvironmentInstanceRequest) (*client.EnvironmentInstanceResponseObject, *http.Response, error) {
	panic("unimplemented")
}

// DeleteEnvironmentInstanceLabels implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) DeleteEnvironmentInstanceLabels(ctx context.Context, environmentInstanceId string) client.ApiDeleteEnvironmentInstanceLabelsRequest {
	panic("unimplemented")
}

// DeleteEnvironmentInstanceLabelsExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) DeleteEnvironmentInstanceLabelsExecute(r client.ApiDeleteEnvironmentInstanceLabelsRequest) (*client.LabelsResponseObject, *http.Response, error) {
	panic("unimplemented")
}

// DeleteEnvironmentInstances implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) DeleteEnvironmentInstances(ctx context.Context) client.ApiDeleteEnvironmentInstancesRequest {
	panic("unimplemented")
}

// DeleteEnvironmentInstancesExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) DeleteEnvironmentInstancesExecute(r client.ApiDeleteEnvironmentInstancesRequest) (*client.EnvironmentInstancesResponseCollection, *http.Response, error) {
	panic("unimplemented")
}

// GetAllEnvironmentInstanceBindings implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) GetAllEnvironmentInstanceBindings(ctx context.Context, environmentInstanceId string) client.ApiGetAllEnvironmentInstanceBindingsRequest {
	panic("unimplemented")
}

// GetAllEnvironmentInstanceBindingsExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) GetAllEnvironmentInstanceBindingsExecute(r client.ApiGetAllEnvironmentInstanceBindingsRequest) (*client.GetAllInstanceBindingsResponse, *http.Response, error) {
	panic("unimplemented")
}

// GetAvailableEnvironments implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) GetAvailableEnvironments(ctx context.Context) client.ApiGetAvailableEnvironmentsRequest {
	panic("unimplemented")
}

// GetAvailableEnvironmentsExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) GetAvailableEnvironmentsExecute(r client.ApiGetAvailableEnvironmentsRequest) (*client.AvailableEnvironmentResponseCollection, *http.Response, error) {
	panic("unimplemented")
}

// GetEnvironmentInstance implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) GetEnvironmentInstance(ctx context.Context, environmentInstanceId string) client.ApiGetEnvironmentInstanceRequest {
	panic("unimplemented")
}

// GetEnvironmentInstanceBinding implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) GetEnvironmentInstanceBinding(ctx context.Context, environmentInstanceId string, bindingId string) client.ApiGetEnvironmentInstanceBindingRequest {
	panic("unimplemented")
}

// GetEnvironmentInstanceBindingExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) GetEnvironmentInstanceBindingExecute(r client.ApiGetEnvironmentInstanceBindingRequest) (*client.GetEnvironmentInstanceBinding200Response, *http.Response, error) {
	panic("unimplemented")
}

// GetEnvironmentInstanceExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) GetEnvironmentInstanceExecute(r client.ApiGetEnvironmentInstanceRequest) (*client.BusinessEnvironmentInstanceResponseObject, *http.Response, error) {
	panic("unimplemented")
}

// GetEnvironmentInstanceLabels implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) GetEnvironmentInstanceLabels(ctx context.Context, environmentInstanceId string) client.ApiGetEnvironmentInstanceLabelsRequest {
	panic("unimplemented")
}

// GetEnvironmentInstanceLabelsExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) GetEnvironmentInstanceLabelsExecute(r client.ApiGetEnvironmentInstanceLabelsRequest) (*client.LabelsResponseObject, *http.Response, error) {
	panic("unimplemented")
}

// GetEnvironmentInstances implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) GetEnvironmentInstances(ctx context.Context) client.ApiGetEnvironmentInstancesRequest {
	return client.ApiGetEnvironmentInstancesRequest{ApiService: m}
}

// GetEnvironmentInstancesExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) GetEnvironmentInstancesExecute(r client.ApiGetEnvironmentInstancesRequest) (*client.BusinessEnvironmentInstancesResponseCollection, *http.Response, error) {
	return m.apiResponse, &http.Response{}, m.err
}

// UpdateEnvironmentInstance implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) UpdateEnvironmentInstance(ctx context.Context, environmentInstanceId string) client.ApiUpdateEnvironmentInstanceRequest {
	panic("unimplemented")
}

// UpdateEnvironmentInstanceExecute implements openapi.EnvironmentsAPI.
func (m *MockProvisioningServiceClient) UpdateEnvironmentInstanceExecute(r client.ApiUpdateEnvironmentInstanceRequest) (map[string]interface{}, *http.Response, error) {
	panic("unimplemented")
}

var _ client.EnvironmentsAPI = &MockProvisioningServiceClient{}
