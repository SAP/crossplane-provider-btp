package di

import (
	"context"

	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
)

// This file contains creator functions for initializers and clients to decouple that logic from controllers and share it across them

func NewPlanIdResolverFn(ctx context.Context, secretData map[string][]byte) (servicemanager.PlanIdResolver, error) {
	binding, err := servicemanager.NewCredsFromOperatorSecret(secretData)
	if err != nil {
		return nil, err
	}
	return servicemanager.NewServiceManagerClient(btp.NewBackgroundContextWithDebugPrintHTTPClient(), &binding)
}
