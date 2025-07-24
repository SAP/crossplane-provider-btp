package kymamodule

import (
	"context"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
)

type Client interface {
	GetDefaultKyma(ctx context.Context) (*v1alpha1.KymaCr, error)
	EnableModule(ctx context.Context, moduleName string, moduleChannel string, customResourcePolicy string) error
	DisableModule(ctx context.Context, moduleName string) error
	// Does not have to be public
	updateDefaultKyma(ctx context.Context, obj *v1alpha1.KymaCr) error
}
