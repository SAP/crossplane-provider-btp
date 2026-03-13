package entitlement

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	entclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
)

// spyClient records call counts so tests can verify caching behaviour.
type spyClient struct {
	describeCalls int
	describeErr   error
	createCalls   int
	createErr     error
	updateCalls   int
	updateErr     error
	deleteCalls   int
	deleteErr     error
}

func (s *spyClient) DescribeInstance(_ context.Context, _ *v1alpha1.Entitlement) (*Instance, error) {
	s.describeCalls++
	if s.describeErr != nil {
		return nil, s.describeErr
	}
	return &Instance{
		EntitledServicePlan: &entclient.ServicePlanResponseObject{Name: internal.Ptr("plan")},
	}, nil
}

func (s *spyClient) CreateInstance(_ context.Context, _ *v1alpha1.Entitlement) error {
	s.createCalls++
	return s.createErr
}

func (s *spyClient) UpdateInstance(_ context.Context, _ *v1alpha1.Entitlement) error {
	s.updateCalls++
	return s.updateErr
}

func (s *spyClient) DeleteInstance(_ context.Context, _ *v1alpha1.Entitlement) error {
	s.deleteCalls++
	return s.deleteErr
}

func testCR() *v1alpha1.Entitlement {
	return &v1alpha1.Entitlement{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-entitlement",
		},
		Spec: v1alpha1.EntitlementSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: "default"},
			},
			ForProvider: v1alpha1.EntitlementParameters{
				SubaccountGuid: "sa-guid",
				ServiceName:    "svc",
				ServicePlanName: "plan",
			},
		},
	}
}

func newTestCachingClient(inner Client) *CachingClient {
	cache := NewInstanceCache(10 * time.Second)
	return NewCachingClient(inner, cache)
}

func TestCachingClient_DescribeInstance_CachesResult(t *testing.T) {
	spy := &spyClient{}
	cc := newTestCachingClient(spy)
	ctx := context.Background()
	cr := testCR()

	result1, err := cc.DescribeInstance(ctx, cr)
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if result1 == nil {
		t.Fatal("first call: expected non-nil result")
	}

	result2, err := cc.DescribeInstance(ctx, cr)
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if result2 != result1 {
		t.Error("second call: expected same pointer (cached), got different instance")
	}

	if spy.describeCalls != 1 {
		t.Errorf("inner DescribeInstance called %d times, want 1", spy.describeCalls)
	}
}

func TestCachingClient_DescribeInstance_PropagatesError(t *testing.T) {
	expectedErr := errors.New("btp api error")
	spy := &spyClient{describeErr: expectedErr}
	cc := newTestCachingClient(spy)

	_, err := cc.DescribeInstance(context.Background(), testCR())
	if !errors.Is(err, expectedErr) {
		t.Errorf("got error %v, want %v", err, expectedErr)
	}
}

func TestCachingClient_CreateInstance_InvalidatesCache(t *testing.T) {
	spy := &spyClient{}
	cc := newTestCachingClient(spy)
	ctx := context.Background()
	cr := testCR()

	// Populate cache
	_, _ = cc.DescribeInstance(ctx, cr)
	if spy.describeCalls != 1 {
		t.Fatalf("setup: expected 1 describe call, got %d", spy.describeCalls)
	}

	// Create invalidates
	if err := cc.CreateInstance(ctx, cr); err != nil {
		t.Fatalf("CreateInstance: unexpected error: %v", err)
	}

	// Next describe should miss cache
	_, _ = cc.DescribeInstance(ctx, cr)
	if spy.describeCalls != 2 {
		t.Errorf("after CreateInstance: inner DescribeInstance called %d times, want 2", spy.describeCalls)
	}
}

func TestCachingClient_UpdateInstance_InvalidatesCache(t *testing.T) {
	spy := &spyClient{}
	cc := newTestCachingClient(spy)
	ctx := context.Background()
	cr := testCR()

	// Populate cache
	_, _ = cc.DescribeInstance(ctx, cr)

	// Update invalidates
	if err := cc.UpdateInstance(ctx, cr); err != nil {
		t.Fatalf("UpdateInstance: unexpected error: %v", err)
	}

	// Next describe should miss cache
	_, _ = cc.DescribeInstance(ctx, cr)
	if spy.describeCalls != 2 {
		t.Errorf("after UpdateInstance: inner DescribeInstance called %d times, want 2", spy.describeCalls)
	}
}

func TestCachingClient_DeleteInstance_InvalidatesCache(t *testing.T) {
	spy := &spyClient{}
	cc := newTestCachingClient(spy)
	ctx := context.Background()
	cr := testCR()

	// Populate cache
	_, _ = cc.DescribeInstance(ctx, cr)

	// Delete invalidates
	if err := cc.DeleteInstance(ctx, cr); err != nil {
		t.Fatalf("DeleteInstance: unexpected error: %v", err)
	}

	// Next describe should miss cache
	_, _ = cc.DescribeInstance(ctx, cr)
	if spy.describeCalls != 2 {
		t.Errorf("after DeleteInstance: inner DescribeInstance called %d times, want 2", spy.describeCalls)
	}
}

func TestCachingClient_MutationError_StillInvalidates(t *testing.T) {
	expectedErr := errors.New("mutation failed")
	spy := &spyClient{createErr: expectedErr}
	cc := newTestCachingClient(spy)
	ctx := context.Background()
	cr := testCR()

	// Populate cache
	_, _ = cc.DescribeInstance(ctx, cr)

	// Create fails but should still invalidate
	err := cc.CreateInstance(ctx, cr)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("CreateInstance: got error %v, want %v", err, expectedErr)
	}

	// Next describe should miss cache (invalidated despite error)
	_, _ = cc.DescribeInstance(ctx, cr)
	if spy.describeCalls != 2 {
		t.Errorf("after failed CreateInstance: inner DescribeInstance called %d times, want 2", spy.describeCalls)
	}
}

func TestRegisterCacheMetrics_RegistersAllMetrics(t *testing.T) {
	cache := NewInstanceCache(10 * time.Second)
	registry := prometheus.NewRegistry()

	// Should not panic
	RegisterCacheMetrics(cache, registry)

	// Gather all registered metrics
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	expected := map[string]dto.MetricType{
		"crossplane_entitlement_cache_hits_total":   dto.MetricType_COUNTER,
		"crossplane_entitlement_cache_misses_total":  dto.MetricType_COUNTER,
		"crossplane_entitlement_cache_hit_ratio":     dto.MetricType_GAUGE,
	}

	found := make(map[string]bool)
	for _, family := range families {
		if wantType, ok := expected[family.GetName()]; ok {
			found[family.GetName()] = true
			if family.GetType() != wantType {
				t.Errorf("metric %s: expected %s type, got %s", family.GetName(), wantType, family.GetType())
			}
		}
	}

	for name := range expected {
		if !found[name] {
			t.Errorf("metric %q not registered", name)
		}
	}
}
