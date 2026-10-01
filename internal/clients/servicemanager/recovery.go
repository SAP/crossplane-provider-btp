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
// external-name recovery path (Lookup*) and by opt-in adoption (Find*).
// Implementations are scoped to one subaccount by their credentials; callers
// still apply their own ownership or identity checks before patching
// external-name.
type SemanticLookuper interface {
	// FindServiceInstance returns the single instance named name.
	FindServiceInstance(ctx context.Context, name string) (InstanceMatch, bool, error)

	// FindServiceBinding returns the single binding named exactly name under
	// the instance. It never matches rotated "<name>-<suffix>" bindings.
	FindServiceBinding(ctx context.Context, serviceInstanceID, name string) (BindingMatch, bool, error)

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
	match, found, err := sm.FindServiceInstance(ctx, name)
	if err != nil {
		return "", time.Time{}, false, err
	}
	return match.ID, match.CreatedAt, found, nil
}

func (sm *ServiceManagerClient) LookupServiceBinding(ctx context.Context, serviceInstanceID, name string) (string, time.Time, bool, error) {
	match, found, err := sm.FindServiceBinding(ctx, serviceInstanceID, name)
	if err != nil {
		return "", time.Time{}, false, err
	}
	if !found {
		return sm.lookupRotatedBinding(ctx, serviceInstanceID, name)
	}
	return match.ID, match.CreatedAt, true, nil
}

func (sm *ServiceManagerClient) lookupRotatedBinding(ctx context.Context, serviceInstanceID, name string) (string, time.Time, bool, error) {
	all, _, err := sm.GetAllServiceBindings(ctx).
		FieldQuery("service_instance_id eq " + quote(serviceInstanceID)).Execute()
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
	instanceQuery := fmt.Sprintf("service_plan_id eq %s and name eq %s", quote(planID), quote(instanceName))

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
			"%d service instances match plan %q name %q in this subaccount",
			len(instanceItems), planID, instanceName)
	}

	instanceID := internal.Val(instanceItems[0].Id)
	instanceCreatedAt := instanceItems[0].GetCreatedAt()

	bindingQuery := fmt.Sprintf("service_instance_id eq %s and name eq %s", quote(instanceID), quote(bindingName))
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
			"%d bindings match name %q for service instance %q",
			len(bindingItems), bindingName, instanceID)
	}
}
