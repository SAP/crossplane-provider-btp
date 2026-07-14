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

// NewSemanticLookuperFn builds a servicemanager.SemanticLookuper from a BTP
// service operator (service manager binding) secret's data. It is used by the
// orphaned-external-name adoption heal path. The returned *ServiceManagerClient
// is scoped to the subaccount the binding belongs to, so semantic lookups are
// implicitly constrained to that subaccount.
func NewSemanticLookuperFn(ctx context.Context, secretData map[string][]byte) (servicemanager.SemanticLookuper, error) {
	binding, err := servicemanager.NewCredsFromOperatorSecret(secretData)
	if err != nil {
		return nil, err
	}
	return servicemanager.NewServiceManagerClient(ctx, &binding)
}
