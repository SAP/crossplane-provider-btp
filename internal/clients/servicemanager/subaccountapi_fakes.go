package servicemanager

import (
	"context"
	"net/http"

	saops "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

var _ saops.SubaccountOperationsAPI = &SubaccountServiceFake{}

type SubaccountServiceFake struct {
	CreateMockFn func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error)
	GetMockFn    func() (*saops.ServiceManagerBindingResponseObject, *http.Response, error)
	DeleteMockFn func() (*http.Response, error)

	AdminBindingDeleteCalled bool
}

func (s *SubaccountServiceFake) CloneNeoSubaccount(ctx context.Context, sourceSubaccountGUID string) saops.ApiCloneNeoSubaccountRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) CloneNeoSubaccountExecute(r saops.ApiCloneNeoSubaccountRequest) (*saops.SubaccountResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) CreateOrUpdateSubaccountSettings(ctx context.Context, subaccountGUID string) saops.ApiCreateOrUpdateSubaccountSettingsRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) CreateOrUpdateSubaccountSettingsExecute(r saops.ApiCreateOrUpdateSubaccountSettingsRequest) (*saops.DataResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) CreateServiceManagementBinding(ctx context.Context, subaccountGUID string) saops.ApiCreateServiceManagementBindingRequest {
	return saops.ApiCreateServiceManagementBindingRequest{ApiService: s}
}

func (s *SubaccountServiceFake) CreateServiceManagementBindingExecute(r saops.ApiCreateServiceManagementBindingRequest) (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
	return s.CreateMockFn()
}

func (s *SubaccountServiceFake) CreateServiceManagerBindingV2(ctx context.Context, subaccountGUID string) saops.ApiCreateServiceManagerBindingV2Request {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) CreateServiceManagerBindingV2Execute(r saops.ApiCreateServiceManagerBindingV2Request) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) CreateSubaccount(ctx context.Context) saops.ApiCreateSubaccountRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) CreateSubaccountExecute(r saops.ApiCreateSubaccountRequest) (*saops.SubaccountResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) CreateSubaccountLabels(ctx context.Context, subaccountGUID string) saops.ApiCreateSubaccountLabelsRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) CreateSubaccountLabelsExecute(r saops.ApiCreateSubaccountLabelsRequest) (*saops.LabelsResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) DeleteServiceManagementBindingOfSubaccount(ctx context.Context, subaccountGUID string) saops.ApiDeleteServiceManagementBindingOfSubaccountRequest {
	return saops.ApiDeleteServiceManagementBindingOfSubaccountRequest{ApiService: s}
}

func (s *SubaccountServiceFake) DeleteServiceManagementBindingOfSubaccountExecute(r saops.ApiDeleteServiceManagementBindingOfSubaccountRequest) (*http.Response, error) {
	s.AdminBindingDeleteCalled = true
	return s.DeleteMockFn()
}

func (s *SubaccountServiceFake) DeleteServiceManagerBindingV2(ctx context.Context, subaccountGUID string, bindingName string) saops.ApiDeleteServiceManagerBindingV2Request {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) DeleteServiceManagerBindingV2Execute(r saops.ApiDeleteServiceManagerBindingV2Request) (*http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) DeleteSubaccount(ctx context.Context, subaccountGUID string) saops.ApiDeleteSubaccountRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) DeleteSubaccountExecute(r saops.ApiDeleteSubaccountRequest) (*saops.SubaccountResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) DeleteSubaccountLabels(ctx context.Context, subaccountGUID string) saops.ApiDeleteSubaccountLabelsRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) DeleteSubaccountLabelsExecute(r saops.ApiDeleteSubaccountLabelsRequest) (*saops.LabelsResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) DeleteSubaccountSettings(ctx context.Context, subaccountGUID string) saops.ApiDeleteSubaccountSettingsRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) DeleteSubaccountSettingsExecute(r saops.ApiDeleteSubaccountSettingsRequest) (*saops.DataResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetAllServiceManagerBindingsV2(ctx context.Context, subaccountGUID string) saops.ApiGetAllServiceManagerBindingsV2Request {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetAllServiceManagerBindingsV2Execute(r saops.ApiGetAllServiceManagerBindingsV2Request) (*saops.ServiceManagerBindingsResponseList, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetServiceManagementBinding(ctx context.Context, subaccountGUID string) saops.ApiGetServiceManagementBindingRequest {
	return saops.ApiGetServiceManagementBindingRequest{ApiService: s}
}

func (s *SubaccountServiceFake) GetServiceManagementBindingExecute(r saops.ApiGetServiceManagementBindingRequest) (*saops.ServiceManagerBindingResponseObject, *http.Response, error) {
	return s.GetMockFn()
}

func (s *SubaccountServiceFake) GetServiceManagerBindingV2(ctx context.Context, subaccountGUID string, bindingName string) saops.ApiGetServiceManagerBindingV2Request {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetServiceManagerBindingV2Execute(r saops.ApiGetServiceManagerBindingV2Request) (*saops.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetSubaccount(ctx context.Context, subaccountGUID string) saops.ApiGetSubaccountRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetSubaccountExecute(r saops.ApiGetSubaccountRequest) (*saops.SubaccountResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetSubaccountCustomProperties(ctx context.Context, subaccountGUID string) saops.ApiGetSubaccountCustomPropertiesRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetSubaccountCustomPropertiesExecute(r saops.ApiGetSubaccountCustomPropertiesRequest) (*saops.ResponseCollection, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetSubaccountLabels(ctx context.Context, subaccountGUID string) saops.ApiGetSubaccountLabelsRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetSubaccountLabelsExecute(r saops.ApiGetSubaccountLabelsRequest) (*saops.LabelsResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetSubaccountSettings(ctx context.Context, subaccountGUID string) saops.ApiGetSubaccountSettingsRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetSubaccountSettingsExecute(r saops.ApiGetSubaccountSettingsRequest) (*saops.DataResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetSubaccounts(ctx context.Context) saops.ApiGetSubaccountsRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) GetSubaccountsExecute(r saops.ApiGetSubaccountsRequest) (*saops.ResponseCollection, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) MoveSubaccount(ctx context.Context, subaccountGUID string) saops.ApiMoveSubaccountRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) MoveSubaccountExecute(r saops.ApiMoveSubaccountRequest) (*saops.SubaccountResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) MoveSubaccounts(ctx context.Context) saops.ApiMoveSubaccountsRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) MoveSubaccountsExecute(r saops.ApiMoveSubaccountsRequest) (*saops.ResponseCollection, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) UpdateSubaccount(ctx context.Context, subaccountGUID string) saops.ApiUpdateSubaccountRequest {
	//TODO implement me
	panic("implement me")
}

func (s *SubaccountServiceFake) UpdateSubaccountExecute(r saops.ApiUpdateSubaccountRequest) (*saops.SubaccountResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}
