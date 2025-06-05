package subaccount

import (
	"context"
	"net/http"

	"github.com/go-openapi/runtime"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

type MockAccountsApiAccessor struct {
	LastMoveTarget string
	returnErr      error
}

func (m *MockAccountsApiAccessor) MoveSubaccount(ctx context.Context, subaccountGuid string, targetId string) error {
	m.LastMoveTarget = targetId
	return m.returnErr
}

func (m *MockAccountsApiAccessor) UpdateSubaccount(ctx context.Context, subaccountGuid string, payload accountclient.UpdateSubaccountRequestPayload) error {
	return m.returnErr
}

var _ AccountsApiAccessor = &MockAccountsApiAccessor{}

type MockSubaccountClient struct {
	returnSubaccounts *accountclient.ResponseCollection
	returnSubaccount  *accountclient.SubaccountResponseObject
	mockDeleteSubaccountExecute  func(r accountclient.ApiDeleteSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error)
	returnErr         error
}

var _ accountclient.SubaccountOperationsAPI = &MockSubaccountClient{}

func (m *MockSubaccountClient) GetSubaccounts(ctx context.Context) accountclient.ApiGetSubaccountsRequest {
	return accountclient.ApiGetSubaccountsRequest{ApiService: m}
}

func (m *MockSubaccountClient) GetSubaccountsExecute(r accountclient.ApiGetSubaccountsRequest) (*accountclient.ResponseCollection, *http.Response, error) {
	return m.returnSubaccounts, nil, m.returnErr
}

func (m *MockSubaccountClient) CreateSubaccount(ctx context.Context) accountclient.ApiCreateSubaccountRequest {
	return accountclient.ApiCreateSubaccountRequest{ApiService: m}
}

func (m *MockSubaccountClient) CreateSubaccountExecute(r accountclient.ApiCreateSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
	return m.returnSubaccount, nil, m.returnErr
}

func (m *MockSubaccountClient) UpdateSubaccount(ctx context.Context, subaccountGUID string) accountclient.ApiUpdateSubaccountRequest {
	return accountclient.ApiUpdateSubaccountRequest{ApiService: m}
}

func (m *MockSubaccountClient) UpdateSubaccountExecute(r accountclient.ApiUpdateSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
	return m.returnSubaccount, nil, m.returnErr
}

func (m *MockSubaccountClient) MoveSubaccount(ctx context.Context, subaccountGUID string) accountclient.ApiMoveSubaccountRequest {
	return accountclient.ApiMoveSubaccountRequest{ApiService: m}
}

func (m *MockSubaccountClient) MoveSubaccountExecute(r accountclient.ApiMoveSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
	return m.returnSubaccount, nil, m.returnErr
}

func (m *MockSubaccountClient) DeleteSubaccount(ctx context.Context, subaccountGUID string) accountclient.ApiDeleteSubaccountRequest {
	return accountclient.ApiDeleteSubaccountRequest{ApiService: m}
}

func (m *MockSubaccountClient) DeleteSubaccountExecute(r accountclient.ApiDeleteSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
	return m.mockDeleteSubaccountExecute(r)
}


func (m *MockSubaccountClient) CloneNeoSubaccount(ctx context.Context, sourceSubaccountGUID string) accountclient.ApiCloneNeoSubaccountRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) CloneNeoSubaccountExecute(r accountclient.ApiCloneNeoSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) CreateOrUpdateSubaccountSettings(ctx context.Context, subaccountGUID string) accountclient.ApiCreateOrUpdateSubaccountSettingsRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) CreateOrUpdateSubaccountSettingsExecute(r accountclient.ApiCreateOrUpdateSubaccountSettingsRequest) (*accountclient.DataResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) CreateServiceManagementBinding(ctx context.Context, subaccountGUID string) accountclient.ApiCreateServiceManagementBindingRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) CreateServiceManagementBindingExecute(r accountclient.ApiCreateServiceManagementBindingRequest) (*accountclient.ServiceManagerBindingResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) CreateServiceManagerBindingV2(ctx context.Context, subaccountGUID string) accountclient.ApiCreateServiceManagerBindingV2Request {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) CreateServiceManagerBindingV2Execute(r accountclient.ApiCreateServiceManagerBindingV2Request) (*accountclient.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) CreateSubaccountLabels(ctx context.Context, subaccountGUID string) accountclient.ApiCreateSubaccountLabelsRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) CreateSubaccountLabelsExecute(r accountclient.ApiCreateSubaccountLabelsRequest) (*accountclient.LabelsResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) DeleteServiceManagementBindingOfSubaccount(ctx context.Context, subaccountGUID string) accountclient.ApiDeleteServiceManagementBindingOfSubaccountRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) DeleteServiceManagementBindingOfSubaccountExecute(r accountclient.ApiDeleteServiceManagementBindingOfSubaccountRequest) (*http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) DeleteServiceManagerBindingV2(ctx context.Context, subaccountGUID string, bindingName string) accountclient.ApiDeleteServiceManagerBindingV2Request {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) DeleteServiceManagerBindingV2Execute(r accountclient.ApiDeleteServiceManagerBindingV2Request) (*http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) DeleteSubaccountLabels(ctx context.Context, subaccountGUID string) accountclient.ApiDeleteSubaccountLabelsRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) DeleteSubaccountLabelsExecute(r accountclient.ApiDeleteSubaccountLabelsRequest) (*accountclient.LabelsResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) DeleteSubaccountSettings(ctx context.Context, subaccountGUID string) accountclient.ApiDeleteSubaccountSettingsRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) DeleteSubaccountSettingsExecute(r accountclient.ApiDeleteSubaccountSettingsRequest) (*accountclient.DataResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetAllServiceManagerBindingsV2(ctx context.Context, subaccountGUID string) accountclient.ApiGetAllServiceManagerBindingsV2Request {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetAllServiceManagerBindingsV2Execute(r accountclient.ApiGetAllServiceManagerBindingsV2Request) (*accountclient.ServiceManagerBindingsResponseList, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetServiceManagementBinding(ctx context.Context, subaccountGUID string) accountclient.ApiGetServiceManagementBindingRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetServiceManagementBindingExecute(r accountclient.ApiGetServiceManagementBindingRequest) (*accountclient.ServiceManagerBindingResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetServiceManagerBindingV2(ctx context.Context, subaccountGUID string, bindingName string) accountclient.ApiGetServiceManagerBindingV2Request {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetServiceManagerBindingV2Execute(r accountclient.ApiGetServiceManagerBindingV2Request) (*accountclient.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetSubaccount(ctx context.Context, subaccountGUID string) accountclient.ApiGetSubaccountRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetSubaccountExecute(r accountclient.ApiGetSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetSubaccountCustomProperties(ctx context.Context, subaccountGUID string) accountclient.ApiGetSubaccountCustomPropertiesRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetSubaccountCustomPropertiesExecute(r accountclient.ApiGetSubaccountCustomPropertiesRequest) (*accountclient.ResponseCollection, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetSubaccountLabels(ctx context.Context, subaccountGUID string) accountclient.ApiGetSubaccountLabelsRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetSubaccountLabelsExecute(r accountclient.ApiGetSubaccountLabelsRequest) (*accountclient.LabelsResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetSubaccountSettings(ctx context.Context, subaccountGUID string) accountclient.ApiGetSubaccountSettingsRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) GetSubaccountSettingsExecute(r accountclient.ApiGetSubaccountSettingsRequest) (*accountclient.DataResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) MoveSubaccounts(ctx context.Context) accountclient.ApiMoveSubaccountsRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) MoveSubaccountsExecute(r accountclient.ApiMoveSubaccountsRequest) (*accountclient.ResponseCollection, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubaccountClient) SetTransport(transport runtime.ClientTransport) {
	//TODO implement me
	panic("implement me")
}
