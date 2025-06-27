package subscription

import (
	"context"
	"net/http"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	saas_client "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-saas-provisioning-api-go/pkg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestSubscriptionApiHandler_GetSubscription(t *testing.T) {
	tests := []struct {
		name                string
		externalName        string
		mockSubscriptionApi *MockSubscriptionOperationsConsumer

		wantErr      error
		wantResponse *saas_client.EntitledApplicationsResponseObject
	}{
		{
			name:         "APIerror",
			externalName: "name1/plan2",
			mockSubscriptionApi: apiMockGET(
				nil,
				500,
				errors.New("apiError"),
			),
			wantErr: errors.New("apiError"),
		},
		{
			name:         "NotFound",
			externalName: "name1/plan2",
			mockSubscriptionApi: apiMockGET(
				nil,
				// right now the api returns 429 as not found...
				429,
				errors.New("notFoundError"),
			),
			wantErr: errors.New("notFoundError"),
		},
		{
			name:         "NotFoundDueNotSubscribed",
			externalName: "name1/plan2",
			mockSubscriptionApi: apiMockGET(
				&saas_client.EntitledApplicationsResponseObject{State: internal.Ptr("UNSUBSCRIBED")},
				200,
				nil,
			),
			wantResponse: &saas_client.EntitledApplicationsResponseObject{State: internal.Ptr("UNSUBSCRIBED")},
			wantErr:      nil,
		},
		{
			name:         "Success",
			externalName: "name1/plan2",
			mockSubscriptionApi: apiMockGET(
				&saas_client.EntitledApplicationsResponseObject{},
				200,
				nil,
			),
			wantErr:      nil,
			wantResponse: &saas_client.EntitledApplicationsResponseObject{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uut := SubscriptionApiHandler{
				client: &saas_client.APIClient{
					SubscriptionOperationsForAppConsumersAPI: tc.mockSubscriptionApi,
				},
			}
			sub, err := uut.GetSubscription(context.TODO(), tc.externalName)

			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nGetSubscription(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.wantResponse, sub); diff != "" {
				t.Errorf("\nGetSubscription(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestSubscriptionApiHandler_CreateSubscription(t *testing.T) {
	tests := []struct {
		name                string
		payload             SubscriptionPost
		mockSubscriptionApi *MockSubscriptionOperationsConsumer

		wantErr      error
		wantResponse string
	}{
		{
			name: "APIerror",
			payload: SubscriptionPost{
				appName: "name1",
				CreateSubscriptionRequestPayload: saas_client.CreateSubscriptionRequestPayload{
					PlanName: internal.Ptr("plan2"),
					SubscriptionParams: map[string]interface{}{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			mockSubscriptionApi: apiMockPOST(
				500,
				errors.New("apiError"),
			),
			wantErr: errors.New("apiError"),
		},
		{
			name: "Success",
			payload: SubscriptionPost{
				appName: "name1",
				CreateSubscriptionRequestPayload: saas_client.CreateSubscriptionRequestPayload{
					PlanName: internal.Ptr("plan2"),
					SubscriptionParams: map[string]interface{}{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			mockSubscriptionApi: apiMockPOST(
				200,
				nil,
			),
			wantErr:      nil,
			wantResponse: "name1/plan2",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uut := SubscriptionApiHandler{
				client: &saas_client.APIClient{
					SubscriptionOperationsForAppConsumersAPI: tc.mockSubscriptionApi,
				},
			}
			sub, err := uut.CreateSubscription(context.TODO(), tc.payload)

			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nGetSubscription(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.wantResponse, sub); diff != "" {
				t.Errorf("\nGetSubscription(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestSubscriptionApiHandler_DeleteSubscription(t *testing.T) {
	tests := []struct {
		name                string
		externalName        string
		mockSubscriptionApi *MockSubscriptionOperationsConsumer

		wantErr error
	}{
		{
			name:         "APIerror",
			externalName: "name1/plan2",
			mockSubscriptionApi: apiMockDELETE(
				500,
				errors.New("apiError"),
			),
			wantErr: errors.New("apiError"),
		},
		{
			name:         "Success",
			externalName: "name1/plan2",
			mockSubscriptionApi: apiMockDELETE(
				200,
				nil,
			),
			wantErr: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uut := SubscriptionApiHandler{
				client: &saas_client.APIClient{
					SubscriptionOperationsForAppConsumersAPI: tc.mockSubscriptionApi,
				},
			}
			err := uut.DeleteSubscription(context.TODO(), tc.externalName)

			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nGetSubscription(...): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}

func TestSubscriptionApiHandler_UpdateSubscription(t *testing.T) {
	tests := []struct {
		name                string
		externalName        string
		payload             SubscriptionPut
		mockSubscriptionApi *MockSubscriptionOperationsConsumer

		wantErr error
	}{
		{
			name:         "APIerror",
			externalName: "name1/plan2",
			payload: SubscriptionPut{
				appName: "name1",
				UpdateSubscriptionRequestPayload: saas_client.UpdateSubscriptionRequestPayload{
					PlanName: internal.Ptr("plan2"),
				},
			},
			mockSubscriptionApi: apiMockPUT(
				500,
				errors.New("apiError"),
			),
			wantErr: errors.New("apiError"),
		},
		{
			name:         "Success",
			externalName: "name1/plan2",
			payload: SubscriptionPut{
				appName: "name1",
				UpdateSubscriptionRequestPayload: saas_client.UpdateSubscriptionRequestPayload{
					PlanName: internal.Ptr("plan2"),
				},
			},
			mockSubscriptionApi: apiMockPUT(
				200,
				nil,
			),
			wantErr: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uut := SubscriptionApiHandler{
				client: &saas_client.APIClient{
					SubscriptionOperationsForAppConsumersAPI: tc.mockSubscriptionApi,
				},
			}

			err := uut.UpdateSubscription(context.TODO(), tc.externalName, tc.payload)

			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nGetSubscription(...): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}

func rawExtension(content string) runtime.RawExtension {
	return runtime.RawExtension{
		Raw: []byte(content),
	}
}

func TestSubscriptionTypeMapper_ConvertToCreatePayload(t *testing.T) {
	raw := rawExtension(`{"name": "John", "age": 30}`)
	cr := NewSubscription("someName", "name1", "plan2", raw)

	uut := NewSubscriptionTypeMapper()
	mapped := uut.ConvertToCreatePayload(cr)

	assert.NotNil(t, mapped)
	assert.Equal(t, "name1", mapped.appName)
	assert.Equal(t, internal.Ptr("plan2"), mapped.PlanName)
	assert.Equal(t, "John", mapped.SubscriptionParams["name"])
	assert.Equal(t, float64(30), mapped.SubscriptionParams["age"])
}

func TestSubscriptionTypeMapper_ConvertToCreatePayloadYaml(t *testing.T) {
	raw := rawExtension(`name: John
age: 30`)
	cr := NewSubscription("someName", "name1", "plan2", raw)

	uut := NewSubscriptionTypeMapper()
	mapped := uut.ConvertToCreatePayload(cr)

	assert.NotNil(t, mapped)
	assert.Equal(t, "name1", mapped.appName)
	assert.Equal(t, internal.Ptr("plan2"), mapped.PlanName)
	assert.Equal(t, "John", mapped.SubscriptionParams["name"])
	assert.Equal(t, float64(30), mapped.SubscriptionParams["age"])
}

func TestSubscriptionTypeMapper_IsSynced(t *testing.T) {
	raw := rawExtension(`{"name": "John", "age": 30}`)
	cr := NewSubscription("someName", "name1", "plan2", raw)
	get := &SubscriptionGet{
		AppName:  internal.Ptr("anotherName"),
		PlanName: internal.Ptr("anotherPlan"),
	}

	uut := NewSubscriptionTypeMapper()
	synced := uut.IsUpToDate(cr, get)

	// since we currently don't support updates, we expect to always return true
	assert.True(t, synced)
}

func TestSubscriptionTypeMapper_IsAvailable(t *testing.T) {
	tests := map[string]struct {
		cr            *v1alpha1.Subscription
		wantAvailable bool
	}{
		"Available": {
			cr: NewSubscriptionWithStatus("someName", "name1", "plan2",
				v1alpha1.SubscriptionObservation{
					State: internal.Ptr("SUBSCRIBED"),
				}),
			wantAvailable: true,
		},
		"UnavailableDifferentState": {
			cr: NewSubscriptionWithStatus("someName", "name1", "plan2",
				v1alpha1.SubscriptionObservation{
					State: internal.Ptr("IN_PROCESS"),
				}),
			wantAvailable: false,
		},
		"UnavailableNoState": {
			cr: NewSubscriptionWithStatus("someName", "name1", "plan2",
				v1alpha1.SubscriptionObservation{
					State: nil,
				}),
			wantAvailable: false,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			uut := NewSubscriptionTypeMapper()
			if uut.IsAvailable(tc.cr) != tc.wantAvailable {
				t.Errorf("Unexpected IsAvailbale, expected: %v", tc.wantAvailable)
			}
		})
	}
}

func TestSubscriptionTypeMapper_SyncStatus(t *testing.T) {
	raw := rawExtension(`{"name": "John", "age": 30}`)
	tests := map[string]struct {
		cr     *v1alpha1.Subscription
		apiRes *SubscriptionGet

		expectedCr *v1alpha1.Subscription
	}{
		"SetState": {
			cr: NewSubscription("someName", "name1", "plan2", raw),
			apiRes: &SubscriptionGet{
				AppName:  internal.Ptr("name1"),
				PlanName: internal.Ptr("plan2"),
				State:    internal.Ptr(v1alpha1.SubscriptionStateInProcess),
			},
			expectedCr: NewSubscriptionWithStatus("someName", "name1", "plan2",
				v1alpha1.SubscriptionObservation{
					State: internal.Ptr(v1alpha1.SubscriptionStateInProcess),
				},
			),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			uut := NewSubscriptionTypeMapper()
			uut.SyncStatus(tc.apiRes, &tc.cr.Status.AtProvider)
			if diff := cmp.Diff(tc.expectedCr.Status.AtProvider, tc.cr.Status.AtProvider); diff != "" {
				t.Errorf("\nSyncState(...): -want, +got:\n%s\n", diff)
			}
		})
	}

}

func apiMockGET(response *saas_client.EntitledApplicationsResponseObject, statusCode int, apiError error) *MockSubscriptionOperationsConsumer {
	apiMock := &MockSubscriptionOperationsConsumer{}
	apiMock.
		On("GetEntitledApplicationExecute", mock.Anything).
		Return(response, &http.Response{StatusCode: statusCode}, apiError)
	return apiMock
}

func apiMockPOST(statusCode int, apiError error) *MockSubscriptionOperationsConsumer {
	apiMock := &MockSubscriptionOperationsConsumer{}
	apiMock.
		On("CreateSubscriptionAsyncExecute", mock.Anything).
		Return(&http.Response{StatusCode: statusCode}, apiError)
	return apiMock
}

func apiMockPUT(statusCode int, apiError error) *MockSubscriptionOperationsConsumer {
	apiMock := &MockSubscriptionOperationsConsumer{}
	apiMock.
		On("UpdateSubscriptionParametersAsyncExecute", mock.Anything).
		Return(&http.Response{StatusCode: statusCode}, apiError)
	return apiMock
}

func apiMockDELETE(statusCode int, apiError error) *MockSubscriptionOperationsConsumer {
	apiMock := &MockSubscriptionOperationsConsumer{}
	apiMock.
		On("DeleteSubscriptionAsyncExecute", mock.Anything).
		Return(&http.Response{StatusCode: statusCode}, apiError)
	return apiMock
}

func NewSubscription(crName string, appName string, planName string, subscriptionParameters runtime.RawExtension) *v1alpha1.Subscription {
	cr := &v1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{Name: crName},
		Spec: v1alpha1.SubscriptionSpec{
			ForProvider: v1alpha1.SubscriptionParameters{
				AppName:                appName,
				PlanName:               planName,
				SubscriptionParameters: subscriptionParameters,
			},
		},
	}
	return cr
}

func NewSubscriptionWithStatus(crName string, appName string, planName string, observation v1alpha1.SubscriptionObservation) *v1alpha1.Subscription {
	cr := &v1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{Name: crName},
		Spec: v1alpha1.SubscriptionSpec{
			ForProvider: v1alpha1.SubscriptionParameters{
				AppName:  appName,
				PlanName: planName,
			},
		},
		Status: v1alpha1.SubscriptionStatus{
			AtProvider: observation,
		},
	}
	return cr
}
