package subscription

import (
	"context"
	"net/http"

	saas_client "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-saas-provisioning-api-go/pkg"
	"github.com/stretchr/testify/mock"
)

var _ saas_client.SubscriptionOperationsForAppConsumersAPI = &MockSubscriptionOperationsConsumer{}

type MockSubscriptionOperationsConsumer struct {
	mock.Mock
}

func (m *MockSubscriptionOperationsConsumer) GetEntitledApplication(ctx context.Context, appName string) saas_client.ApiGetEntitledApplicationRequest {
	return saas_client.ApiGetEntitledApplicationRequest{ApiService: m}
}

func (m *MockSubscriptionOperationsConsumer) GetEntitledApplicationExecute(r saas_client.ApiGetEntitledApplicationRequest) (*saas_client.EntitledApplicationsResponseObject, *http.Response, error) {
	args := m.Called(r)
	returnedErr, _ := args.Get(2).(error)
	return args.Get(0).(*saas_client.EntitledApplicationsResponseObject),
		args.Get(1).(*http.Response),
		returnedErr
}

func (m *MockSubscriptionOperationsConsumer) CreateSubscriptionAsync(ctx context.Context, appName string) saas_client.ApiCreateSubscriptionAsyncRequest {
	return saas_client.ApiCreateSubscriptionAsyncRequest{ApiService: m}
}

func (m *MockSubscriptionOperationsConsumer) CreateSubscriptionAsyncExecute(r saas_client.ApiCreateSubscriptionAsyncRequest) (*http.Response, error) {
	args := m.Called(r)
	returnedErr, _ := args.Get(1).(error)
	return args.Get(0).(*http.Response), returnedErr
}

func (m *MockSubscriptionOperationsConsumer) DeleteSubscriptionAsync(ctx context.Context, appName string) saas_client.ApiDeleteSubscriptionAsyncRequest {
	return saas_client.ApiDeleteSubscriptionAsyncRequest{ApiService: m}
}

func (m *MockSubscriptionOperationsConsumer) DeleteSubscriptionAsyncExecute(r saas_client.ApiDeleteSubscriptionAsyncRequest) (*http.Response, error) {
	args := m.Called(r)
	returnedErr, _ := args.Get(1).(error)
	return args.Get(0).(*http.Response), returnedErr
}

func (m *MockSubscriptionOperationsConsumer) UpdateSubscriptionParametersAsync(ctx context.Context, appName string) saas_client.ApiUpdateSubscriptionParametersAsyncRequest {
	return saas_client.ApiUpdateSubscriptionParametersAsyncRequest{ApiService: m}
}

func (m *MockSubscriptionOperationsConsumer) UpdateSubscriptionParametersAsyncExecute(r saas_client.ApiUpdateSubscriptionParametersAsyncRequest) (*http.Response, error) {
	args := m.Called(r)
	returnedErr, _ := args.Get(1).(error)
	return args.Get(0).(*http.Response), returnedErr
}

func (m *MockSubscriptionOperationsConsumer) DeleteSubscriptionLabels(ctx context.Context, appName string) saas_client.ApiDeleteSubscriptionLabelsRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubscriptionOperationsConsumer) DeleteSubscriptionLabelsExecute(r saas_client.ApiDeleteSubscriptionLabelsRequest) (map[string]interface{}, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubscriptionOperationsConsumer) GetEntitledApplications(ctx context.Context) saas_client.ApiGetEntitledApplicationsRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubscriptionOperationsConsumer) GetEntitledApplicationsExecute(r saas_client.ApiGetEntitledApplicationsRequest) (*saas_client.EntitledApplicationsResponseCollection, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubscriptionOperationsConsumer) GetSubscriptionLabels(ctx context.Context, appName string) saas_client.ApiGetSubscriptionLabelsRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubscriptionOperationsConsumer) GetSubscriptionLabelsExecute(r saas_client.ApiGetSubscriptionLabelsRequest) (map[string]interface{}, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubscriptionOperationsConsumer) UpsertSubscriptionLabels(ctx context.Context, appName string) saas_client.ApiUpsertSubscriptionLabelsRequest {
	//TODO implement me
	panic("implement me")
}

func (m *MockSubscriptionOperationsConsumer) UpsertSubscriptionLabelsExecute(r saas_client.ApiUpsertSubscriptionLabelsRequest) (map[string]interface{}, *http.Response, error) {
	//TODO implement me
	panic("implement me")
}

// GetSubscriptionParams implements openapi.SubscriptionOperationsForAppConsumersAPI.
func (m *MockSubscriptionOperationsConsumer) GetSubscriptionParams(ctx context.Context, appName string) saas_client.ApiGetSubscriptionParamsRequest {
	panic("unimplemented")
}

// GetSubscriptionParamsExecute implements openapi.SubscriptionOperationsForAppConsumersAPI.
func (m *MockSubscriptionOperationsConsumer) GetSubscriptionParamsExecute(r saas_client.ApiGetSubscriptionParamsRequest) (map[string]interface{}, *http.Response, error) {
	panic("unimplemented")
}
