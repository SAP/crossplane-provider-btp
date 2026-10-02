package servicemanager

import (
	"context"

	"github.com/sap/crossplane-provider-btp/internal"
)

// ServiceInstanceRef identifies a service instance for operator-facing
// diagnostics. It carries only what is needed to name an instance in an error
// message; it is not a general-purpose representation of a service instance.
type ServiceInstanceRef struct {
	ID            string
	Name          string
	ServicePlanID string
	Ready         bool
}

// InstanceLister lists the service instances visible to the credentials the
// implementation is scoped to, which is exactly one subaccount.
//
// It is deliberately a separate interface from SemanticLookuper: adding the
// method there would force a change on every existing implementer and fake,
// and the two are used on entirely different paths.
type InstanceLister interface {
	ListServiceInstances(ctx context.Context) ([]ServiceInstanceRef, error)
}

var _ InstanceLister = &ServiceManagerClient{}

// ListServiceInstances returns every service instance in the subaccount the
// client's credentials are scoped to. It is a read-only call, used to name
// what blocks a refused subaccount deletion.
func (sm *ServiceManagerClient) ListServiceInstances(ctx context.Context) ([]ServiceInstanceRef, error) {
	list, _, err := sm.GetAllServiceInstances(ctx).Execute()
	if err != nil {
		return nil, specifyAPIError(err)
	}

	items := list.GetItems()
	refs := make([]ServiceInstanceRef, 0, len(items))
	for i := range items {
		refs = append(refs, ServiceInstanceRef{
			ID:            internal.Val(items[i].Id),
			Name:          internal.Val(items[i].Name),
			ServicePlanID: internal.Val(items[i].ServicePlanId),
			Ready:         items[i].GetReady(),
		})
	}
	return refs, nil
}
