package directory

import (
	"context"
	"net/http"

	"github.com/go-openapi/runtime"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

type MockDirClient struct {
	GetResult *accountclient.DirectoryResponseObject
	GetErr    error

	CreateResult *accountclient.DirectoryResponseObject
	CreateErr    error

	UpdateErr         error
	UpdateSettingsErr error

	DeleteErr error

	ResultStatusCode int

	// GetNilResponse models a transport-level failure, where the generated client
	// returns a nil *http.Response alongside the error instead of a real response.
	GetNilResponse bool
}

var _ accountclient.DirectoryOperationsAPI = MockDirClient{}

func (m MockDirClient) CreateDirectory(ctx context.Context) accountclient.ApiCreateDirectoryRequest {
	return accountclient.ApiCreateDirectoryRequest{ApiService: m}
}

func (m MockDirClient) CreateDirectoryExecute(r accountclient.ApiCreateDirectoryRequest) (*accountclient.DirectoryResponseObject, *http.Response, error) {
	return m.CreateResult, nil, m.CreateErr
}

func (m MockDirClient) DeleteDirectory(ctx context.Context, directoryGUID string) accountclient.ApiDeleteDirectoryRequest {
	return accountclient.ApiDeleteDirectoryRequest{ApiService: m}
}

func (m MockDirClient) DeleteDirectoryExecute(r accountclient.ApiDeleteDirectoryRequest) (*accountclient.DirectoryResponseObject, *http.Response, error) {
	return nil, nil, m.DeleteErr
}

func (m MockDirClient) GetDirectory(ctx context.Context, directoryGUID string) accountclient.ApiGetDirectoryRequest {
	return accountclient.ApiGetDirectoryRequest{ApiService: m}
}

func (m MockDirClient) GetDirectoryExecute(r accountclient.ApiGetDirectoryRequest) (*accountclient.DirectoryResponseObject, *http.Response, error) {
	if m.GetNilResponse {
		return nil, nil, m.GetErr
	}
	return m.GetResult, &http.Response{StatusCode: m.ResultStatusCode}, m.GetErr
}

func (m MockDirClient) UpdateDirectory(ctx context.Context, directoryGUID string) accountclient.ApiUpdateDirectoryRequest {
	return accountclient.ApiUpdateDirectoryRequest{ApiService: m}
}

func (m MockDirClient) UpdateDirectoryExecute(r accountclient.ApiUpdateDirectoryRequest) (*accountclient.DirectoryResponseObject, *http.Response, error) {
	return nil, nil, m.UpdateErr
}

func (m MockDirClient) UpdateDirectoryFeatures(ctx context.Context, directoryGUID string) accountclient.ApiUpdateDirectoryFeaturesRequest {
	return accountclient.ApiUpdateDirectoryFeaturesRequest{ApiService: m}
}

func (m MockDirClient) UpdateDirectoryFeaturesExecute(r accountclient.ApiUpdateDirectoryFeaturesRequest) (*accountclient.DirectoryResponseObject, *http.Response, error) {
	return nil, nil, m.UpdateSettingsErr
}

func (m MockDirClient) CreateDirectoryLabels(ctx context.Context, directoryGUID string) accountclient.ApiCreateDirectoryLabelsRequest {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) CreateDirectoryLabelsExecute(r accountclient.ApiCreateDirectoryLabelsRequest) (*accountclient.LabelsResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) CreateOrUpdateDirectorySettings(ctx context.Context, directoryGUID string) accountclient.ApiCreateOrUpdateDirectorySettingsRequest {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) CreateOrUpdateDirectorySettingsExecute(r accountclient.ApiCreateOrUpdateDirectorySettingsRequest) (*accountclient.DataResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) DeleteDirectoryLabels(ctx context.Context, directoryGUID string) accountclient.ApiDeleteDirectoryLabelsRequest {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) DeleteDirectoryLabelsExecute(r accountclient.ApiDeleteDirectoryLabelsRequest) (*accountclient.LabelsResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) DeleteDirectorySettings(ctx context.Context, directoryGUID string) accountclient.ApiDeleteDirectorySettingsRequest {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) DeleteDirectorySettingsExecute(r accountclient.ApiDeleteDirectorySettingsRequest) (*accountclient.DataResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) GetDirectoryCustomProperties(ctx context.Context, directoryGUID string) accountclient.ApiGetDirectoryCustomPropertiesRequest {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) GetDirectoryCustomPropertiesExecute(r accountclient.ApiGetDirectoryCustomPropertiesRequest) (*accountclient.ResponseCollection, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) GetDirectoryLabels(ctx context.Context, directoryGUID string) accountclient.ApiGetDirectoryLabelsRequest {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) GetDirectoryLabelsExecute(r accountclient.ApiGetDirectoryLabelsRequest) (*accountclient.LabelsResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) GetDirectorySettings(ctx context.Context, directoryGUID string) accountclient.ApiGetDirectorySettingsRequest {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) GetDirectorySettingsExecute(r accountclient.ApiGetDirectorySettingsRequest) (*accountclient.DataResponseObject, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m MockDirClient) SetTransport(transport runtime.ClientTransport) {
	//TODO implement me
	panic("implement me")
}
