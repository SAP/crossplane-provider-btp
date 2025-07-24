package kymamodule

import (
	"context"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
)

type Client interface {
	DescribeInstance(ctx context.Context, name string) (v1alpha1.KymaModuleObservation, error)
	CreateInstance(ctx context.Context, name string, channel string) error
	// UpdateInstance updates the channel of the KymaModule instance.
	DeleteInstance(ctx context.Context, name string) error
}
