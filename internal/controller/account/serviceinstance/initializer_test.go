package serviceinstance

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errSecret       = errors.New("secret error")
	errNewResolver  = errors.New("new resolver error")
	errApi          = errors.New("api error")
	errStatusUpdate = errors.New("status update error")
)

func TestServicePlanInitializer_Initialize(t *testing.T) {
	testPlanID := "test-plan-id"

	type want struct {
		err    error
		planID string
	}

	tests := map[string]struct {
		mg              resource.Managed
		kube            client.Client
		loadSecretFn    func(client.Client, context.Context, string, string) (map[string][]byte, error)
		newIdResolverFn func(context.Context, map[string][]byte) (smClient.PlanIdResolver, error)
		want            want
	}{
		"already initialized": {
			mg: expectedServiceInstance(
				withObservationData("", "plan-id"),
			),
			want: want{
				planID: "plan-id",
				err:    nil,
			},
		},
		"loadSecret fails": {
			mg: &v1alpha1.ServiceInstance{},
			loadSecretFn: func(kube client.Client, ctx context.Context, name, ns string) (map[string][]byte, error) {
				return nil, errSecret
			},
			want: want{
				err: errSecret,
			},
		},
		"idResolver fails": {
			mg: &v1alpha1.ServiceInstance{},
			loadSecretFn: func(kube client.Client, ctx context.Context, name, ns string) (map[string][]byte, error) {
				return map[string][]byte{}, nil
			},
			newIdResolverFn: func(context.Context, map[string][]byte) (smClient.PlanIdResolver, error) {
				return nil, errNewResolver
			},
			want: want{
				err: errNewResolver,
			},
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
				return &mockPlanIdResolver{"", errApi}, nil
			},
			want: want{
				err: errApi,
			},
		},
		"save status fails": {
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
			kube: &test.MockClient{
				MockStatusUpdate: func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					return errStatusUpdate
				},
			},
			want: want{
				planID: testPlanID,
				err:    errStatusUpdate,
			},
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
			kube: &test.MockClient{
				MockStatusUpdate: func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					return nil
				},
			},
			want: want{
				planID: testPlanID,
				err:    nil,
			},
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

			err := init.Initialize(tc.kube, context.Background(), tc.mg)

			// Check if the error matches the expected error
			expectedErrorBehaviour(t, tc.want.err, err)

			// check if planID has been resolved as expected
			expectedCr := tc.mg.DeepCopyObject()
			expectedCr.(*v1alpha1.ServiceInstance).Status.AtProvider.ServiceplanID = tc.want.planID

			if diff := cmp.Diff(expectedCr, tc.mg); diff != "" {
				t.Errorf("\nCR mismatch (-want, +got):\n%s\n", diff)
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
