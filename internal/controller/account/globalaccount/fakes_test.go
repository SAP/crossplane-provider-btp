package globalaccount

import (
	"context"
	"net/http"

	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

type mockGlobalAccountClient struct {
	returnErr error
}

var _ accountclient.GlobalAccountOperationsAPI = &mockGlobalAccountClient{}

func (m *mockGlobalAccountClient) GetGlobalAccount(ctx context.Context) accountclient.ApiGetGlobalAccountRequest {
	return accountclient.ApiGetGlobalAccountRequest{ApiService: m}
}

func (m *mockGlobalAccountClient) GetGlobalAccountExecute(r accountclient.ApiGetGlobalAccountRequest) (*accountclient.GlobalAccountResponseObject, *http.Response, error) {
	return nil, nil, m.returnErr
}

func (m *mockGlobalAccountClient) GetGlobalAccountCustomProperties(ctx context.Context) accountclient.ApiGetGlobalAccountCustomPropertiesRequest {
	panic("not implemented")
}

func (m *mockGlobalAccountClient) GetGlobalAccountCustomPropertiesExecute(r accountclient.ApiGetGlobalAccountCustomPropertiesRequest) (*accountclient.ResponseCollection, *http.Response, error) {
	panic("not implemented")
}

func (m *mockGlobalAccountClient) UpdateGlobalAccount(ctx context.Context) accountclient.ApiUpdateGlobalAccountRequest {
	panic("not implemented")
}

func (m *mockGlobalAccountClient) UpdateGlobalAccountExecute(r accountclient.ApiUpdateGlobalAccountRequest) (*accountclient.GlobalAccountResponseObject, *http.Response, error) {
	panic("not implemented")
}
