package subscription

import (
	"context"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/subscription"
)

type MockApiHandler struct {
	deleteCounter      int
	returnExternalName string
	returnGet          *subscription.SubscriptionGet
	returnErr          error
}

func (m *MockApiHandler) CreateSubscription(ctx context.Context, payload subscription.SubscriptionPost) (string, error) {
	return m.returnExternalName, m.returnErr
}

func (m *MockApiHandler) UpdateSubscription(ctx context.Context, externalName string, payload subscription.SubscriptionPut) error {
	return m.returnErr
}

func (m *MockApiHandler) DeleteSubscription(ctx context.Context, externalName string) error {
	m.deleteCounter += 1
	return m.returnErr
}

func (m *MockApiHandler) GetSubscription(ctx context.Context, externalName string) (*subscription.SubscriptionGet, error) {
	return m.returnGet, m.returnErr
}

var _ subscription.SubscriptionApiHandlerI = &MockApiHandler{}

type MockTypeMapper struct {
	synced    bool
	available bool
	deletable bool
}

func (m *MockTypeMapper) IsAvailable(cr *v1alpha1.Subscription) bool {
	return m.available
}

func (m *MockTypeMapper) IsDeletable(cr *v1alpha1.Subscription) bool {
	return m.deletable
}

func (m *MockTypeMapper) SyncStatus(get *subscription.SubscriptionGet, crStatus *v1alpha1.SubscriptionObservation) {
	crStatus.State = get.State
}

func (m *MockTypeMapper) ConvertToCreatePayload(cr *v1alpha1.Subscription) subscription.SubscriptionPost {
	return subscription.SubscriptionPost{}
}

func (m *MockTypeMapper) ConvertToUpdatePayload(cr *v1alpha1.Subscription) subscription.SubscriptionPut {
	return subscription.SubscriptionPut{}
}

func (m *MockTypeMapper) IsUpToDate(cr *v1alpha1.Subscription, get *subscription.SubscriptionGet) bool {
	return m.synced
}

var _ subscription.SubscriptionTypeMapperI = &MockTypeMapper{}
