package servicemanager

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/sap/crossplane-provider-btp/internal"
	smclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
)

// SemanticLookuper performs the semantic lookups used by the orphaned
// external-name recovery path. Implementations are scoped to one subaccount by
// their credentials; callers still apply the ownership check from
// internal/recovery before patching external-name.
type SemanticLookuper interface {
	LookupServiceInstance(ctx context.Context, name string) (guid string, createdAt time.Time, found bool, err error)

	// LookupServiceBinding falls back to the single ready rotated binding named
	// "<name>-<random>" when no exact name match exists.
	LookupServiceBinding(ctx context.Context, serviceInstanceID, name string) (guid string, createdAt time.Time, found bool, err error)

	// LookupInstanceAndBinding returns the managed instance plus its managed
	// binding. The instance name disambiguates from the subaccount-admin access
	// instance, and the binding name avoids returning a transient admin binding.
	// The returned created_at is the instance's timestamp (phase-1 happens first).
	LookupInstanceAndBinding(ctx context.Context, planID, instanceName, bindingName string) (serviceInstanceID, serviceBindingID string, instanceCreatedAt time.Time, found bool, err error)
}

var _ SemanticLookuper = &ServiceManagerClient{}

func (sm *ServiceManagerClient) LookupServiceInstance(ctx context.Context, name string) (string, time.Time, bool, error) {
	query := fmt.Sprintf("name eq '%s'", name)

	list, _, err := sm.GetAllServiceInstances(ctx).FieldQuery(query).Execute()
	if err != nil {
		return "", time.Time{}, false, specifyAPIError(err)
	}

	items := list.GetItems()
	switch len(items) {
	case 0:
		return "", time.Time{}, false, nil
	case 1:
		return internal.Val(items[0].Id), items[0].GetCreatedAt(), true, nil
	default:
		return "", time.Time{}, false, errors.Errorf(
			"refusing to recover: %d service instances match name %q in this subaccount", len(items), name)
	}
}

func (sm *ServiceManagerClient) LookupServiceBinding(ctx context.Context, serviceInstanceID, name string) (string, time.Time, bool, error) {
	query := fmt.Sprintf("service_instance_id eq '%s' and name eq '%s'", serviceInstanceID, name)
	list, _, err := sm.GetAllServiceBindings(ctx).FieldQuery(query).Execute()
	if err != nil {
		return "", time.Time{}, false, specifyAPIError(err)
	}
	items := list.GetItems()
	switch len(items) {
	case 0:
		return sm.lookupRotatedBinding(ctx, serviceInstanceID, name)
	case 1:
		return internal.Val(items[0].Id), items[0].GetCreatedAt(), true, nil
	default:
		return "", time.Time{}, false, errors.Errorf(
			"refusing to recover: %d service bindings match name %q for service instance %q",
			len(items), name, serviceInstanceID)
	}
}

func (sm *ServiceManagerClient) lookupRotatedBinding(ctx context.Context, serviceInstanceID, name string) (string, time.Time, bool, error) {
	all, _, err := sm.GetAllServiceBindings(ctx).
		FieldQuery(fmt.Sprintf("service_instance_id eq '%s'", serviceInstanceID)).Execute()
	if err != nil {
		return "", time.Time{}, false, specifyAPIError(err)
	}
	prefix := name + "-"
	var ready []smclient.ListedServiceBindingResponseObject
	for _, it := range all.GetItems() {
		if strings.HasPrefix(it.GetName(), prefix) && it.GetReady() {
			ready = append(ready, it)
		}
	}
	switch len(ready) {
	case 0:
		return "", time.Time{}, false, nil
	case 1:
		return internal.Val(ready[0].Id), ready[0].GetCreatedAt(), true, nil
	default:
		return "", time.Time{}, false, errors.Errorf(
			"refusing to recover: %d ready rotated bindings match prefix %q for service instance %q",
			len(ready), prefix, serviceInstanceID)
	}
}

func (sm *ServiceManagerClient) LookupInstanceAndBinding(ctx context.Context, planID, instanceName, bindingName string) (string, string, time.Time, bool, error) {
	instanceQuery := fmt.Sprintf("service_plan_id eq '%s' and name eq '%s'", planID, instanceName)

	instances, _, err := sm.GetAllServiceInstances(ctx).FieldQuery(instanceQuery).Execute()
	if err != nil {
		return "", "", time.Time{}, false, specifyAPIError(err)
	}

	instanceItems := instances.GetItems()
	switch len(instanceItems) {
	case 0:
		return "", "", time.Time{}, false, nil
	case 1:
		// proceed
	default:
		return "", "", time.Time{}, false, errors.Errorf(
			"refusing to recover: %d service instances match plan %q name %q in this subaccount",
			len(instanceItems), planID, instanceName)
	}

	instanceID := internal.Val(instanceItems[0].Id)
	instanceCreatedAt := instanceItems[0].GetCreatedAt()

	bindingQuery := fmt.Sprintf("service_instance_id eq '%s' and name eq '%s'", instanceID, bindingName)
	bindings, _, err := sm.GetAllServiceBindings(ctx).FieldQuery(bindingQuery).Execute()
	if err != nil {
		return "", "", time.Time{}, false, specifyAPIError(err)
	}

	bindingItems := bindings.GetItems()
	switch len(bindingItems) {
	case 0:
		return instanceID, "", instanceCreatedAt, true, nil
	case 1:
		return instanceID, internal.Val(bindingItems[0].Id), instanceCreatedAt, true, nil
	default:
		return "", "", time.Time{}, false, errors.Errorf(
			"refusing to recover: %d bindings match name %q for service instance %q",
			len(bindingItems), bindingName, instanceID)
	}
}
