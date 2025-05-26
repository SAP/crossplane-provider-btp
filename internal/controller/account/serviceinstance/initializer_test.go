package serviceinstance

import (
	"context"
	"errors"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestServicePlanInitializer_Initialize(t *testing.T) {
	testPlanID := "test-plan-id"
	tests := map[string]struct {
		mg              resource.Managed
		isInitialized   bool
		loadSecretFn    func(client.Client, context.Context, string, string) (map[string][]byte, error)
		newIdResolverFn func(context.Context, map[string][]byte) (smClient.PlanIdResolver, error)
		wantErr         bool
		wantPlanID      *string
	}{
		"not a ServiceInstance": {
			mg:      &struct{ resource.Managed }{},
			wantErr: true,
		},
		"already initialized": {
			mg:            &v1alpha1.ServiceInstance{},
			isInitialized: true,
			wantErr:       false,
		},
		"loadSecret fails": {
			mg: &v1alpha1.ServiceInstance{},
			loadSecretFn: func(kube client.Client, ctx context.Context, name, ns string) (map[string][]byte, error) {
				return nil, errors.New("secret error")
			},
			wantErr: true,
		},
		"idResolver fails": {
			mg: &v1alpha1.ServiceInstance{},
			loadSecretFn: func(kube client.Client, ctx context.Context, name, ns string) (map[string][]byte, error) {
				return map[string][]byte{}, nil
			},
			newIdResolverFn: func(context.Context, map[string][]byte) (smClient.PlanIdResolver, error) {
				return nil, errors.New("resolver error")
			},
			wantErr: true,
		},
		"planID lookup fails": {
			mg: &v1alpha1.ServiceInstance{
				Spec: v1alpha1.ServiceInstanceSpec{
					ForProvider: v1alpha1.ServiceInstanceParameters{},
				},
			},
			loadSecretFn: func(kube client.Client, ctx context.Context, name, ns string) (map[string][]byte, error) {
				return map[string][]byte{}, nil
			},
			newIdResolverFn: func(context.Context, map[string][]byte) (smClient.PlanIdResolver, error) {
				return &mockPlanIdResolver{"", errors.New("lookup error")}, nil
			},
			wantErr: true,
		},
		"success": {
			mg: &v1alpha1.ServiceInstance{
				Spec: v1alpha1.ServiceInstanceSpec{
					ForProvider: v1alpha1.ServiceInstanceParameters{},
				},
			},
			loadSecretFn: func(kube client.Client, ctx context.Context, name, ns string) (map[string][]byte, error) {
				return map[string][]byte{}, nil
			},
			newIdResolverFn: func(context.Context, map[string][]byte) (smClient.PlanIdResolver, error) {
				return &mockPlanIdResolver{testPlanID, nil}, nil
			},
			wantPlanID: &testPlanID,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			init := &servicePlanInitializer{
				loadSecretFn: func(kube client.Client, ctx context.Context, name, ns string) (map[string][]byte, error) {
					if tc.loadSecretFn != nil {
						return tc.loadSecretFn(kube, ctx, name, ns)
					}
					return map[string][]byte{}, nil
				},
				newIdResolverFn: func(ctx context.Context, secretData map[string][]byte) (smClient.PlanIdResolver, error) {
					if tc.newIdResolverFn != nil {
						return tc.newIdResolverFn(ctx, secretData)
					}
					return &mockPlanIdResolver{testPlanID, nil}, nil
				},
			}

			if si, ok := tc.mg.(*v1alpha1.ServiceInstance); ok && tc.isInitialized {
				si.Spec.ForProvider.ServiceplanID = new(string)
			}

			err := init.Initialize(nil, context.Background(), tc.mg)
			if (err != nil) != tc.wantErr {
				t.Errorf("got error = %v, wantErr %v", err, tc.wantErr)
			}
			if si, ok := tc.mg.(*v1alpha1.ServiceInstance); ok && tc.wantPlanID != nil {
				if si.Spec.ForProvider.ServiceplanID == nil || *si.Spec.ForProvider.ServiceplanID != *tc.wantPlanID {
					t.Errorf("got planID = %v, want %v", si.Spec.ForProvider.ServiceplanID, *tc.wantPlanID)
				}
			}
		})
	}
}

type mockPlanIdResolver struct {
	planID string
	err    error
}

func (m *mockPlanIdResolver) PlanIDByName(ctx context.Context, offeringName, planName string) (string, error) {
	return m.planID, m.err
}
