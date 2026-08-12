package fake

import (
	"context"

	apisv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/entitlement"
)

type MockClient struct {
	MockDescribeInstanceFn      func(ctx context.Context, key entitlement.ExternalNameKey) (*entitlement.Instance, error)
	MockDescribeInstanceFreshFn func(ctx context.Context, key entitlement.ExternalNameKey) (*entitlement.Instance, error)
	MockCreateInstanceFn        func(ctx context.Context, key entitlement.ExternalNameKey, cr *apisv1alpha1.Entitlement) error
	MockUpdateInstanceFn        func(ctx context.Context, key entitlement.ExternalNameKey, cr *apisv1alpha1.Entitlement) error
	MockDeleteInstanceFn        func(ctx context.Context, key entitlement.ExternalNameKey, cr *apisv1alpha1.Entitlement) error
}

func (c MockClient) DescribeInstance(ctx context.Context, key entitlement.ExternalNameKey) (
	*entitlement.Instance,
	error,
) {
	if c.MockDescribeInstanceFn != nil {
		return c.MockDescribeInstanceFn(ctx, key)
	}
	return nil, nil
}

// DescribeInstanceFresh delegates to MockDescribeInstanceFreshFn when set, and
// otherwise falls back to MockDescribeInstanceFn so fakes that don't care
// about freshness keep working unchanged.
func (c MockClient) DescribeInstanceFresh(ctx context.Context, key entitlement.ExternalNameKey) (
	*entitlement.Instance,
	error,
) {
	if c.MockDescribeInstanceFreshFn != nil {
		return c.MockDescribeInstanceFreshFn(ctx, key)
	}
	if c.MockDescribeInstanceFn != nil {
		return c.MockDescribeInstanceFn(ctx, key)
	}
	return nil, nil
}
func (c MockClient) CreateInstance(ctx context.Context, key entitlement.ExternalNameKey, cr *apisv1alpha1.Entitlement) error {
	if c.MockCreateInstanceFn != nil {
		return c.MockCreateInstanceFn(ctx, key, cr)
	}
	return nil
}
func (c MockClient) UpdateInstance(ctx context.Context, key entitlement.ExternalNameKey, cr *apisv1alpha1.Entitlement) error {
	if c.MockUpdateInstanceFn != nil {
		return c.MockUpdateInstanceFn(ctx, key, cr)
	}
	return nil
}
func (c MockClient) DeleteInstance(ctx context.Context, key entitlement.ExternalNameKey, cr *apisv1alpha1.Entitlement) error {
	if c.MockDeleteInstanceFn != nil {
		return c.MockDeleteInstanceFn(ctx, key, cr)
	}
	return nil
}
