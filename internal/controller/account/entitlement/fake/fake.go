package fake

import (
	"context"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/entitlement"
)

type MockClient struct {
	MockDescribeCluster  func(ctx context.Context, input apisv1alpha1.Entitlement) (*entitlement.Instance, error)
	MockDeleteInstanceFn func(ctx context.Context, cr *apisv1alpha1.Entitlement) error
}

func (c MockClient) DescribeInstance(ctx context.Context, cr *apisv1alpha1.Entitlement) (
	*entitlement.Instance,
	error,
) {
	return c.MockDescribeCluster(ctx, *cr)
}
func (c MockClient) CreateInstance(ctx context.Context, cr *apisv1alpha1.Entitlement) error {
	return nil
}
func (c MockClient) UpdateInstance(ctx context.Context, cr *apisv1alpha1.Entitlement) error {
	return nil
}
func (c MockClient) DeleteInstance(ctx context.Context, cr *apisv1alpha1.Entitlement) error {
	if c.MockDeleteInstanceFn != nil {
		return c.MockDeleteInstanceFn(ctx, cr)
	}
	return nil
}
