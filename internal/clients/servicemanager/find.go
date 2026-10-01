package servicemanager

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/sap/crossplane-provider-btp/internal"
)

// InstanceMatch identifies the service instance a lookup found.
type InstanceMatch struct {
	ID        string
	PlanID    string
	CreatedAt time.Time
}

// BindingMatch identifies the service binding a lookup found.
type BindingMatch struct {
	ID        string
	CreatedAt time.Time
}

func (sm *ServiceManagerClient) FindServiceInstance(ctx context.Context, name string) (InstanceMatch, bool, error) {
	query := "name eq " + quote(name)

	list, _, err := sm.GetAllServiceInstances(ctx).FieldQuery(query).Execute()
	if err != nil {
		return InstanceMatch{}, false, specifyAPIError(err)
	}

	items := list.GetItems()
	switch len(items) {
	case 0:
		return InstanceMatch{}, false, nil
	case 1:
		match := InstanceMatch{
			ID:        internal.Val(items[0].Id),
			PlanID:    items[0].GetServicePlanId(),
			CreatedAt: items[0].GetCreatedAt(),
		}
		return match, true, nil
	default:
		return InstanceMatch{}, false, errors.Errorf(
			"%d service instances match name %q in this subaccount", len(items), name)
	}
}

func (sm *ServiceManagerClient) FindServiceBinding(ctx context.Context, serviceInstanceID, name string) (BindingMatch, bool, error) {
	query := fmt.Sprintf("service_instance_id eq %s and name eq %s", quote(serviceInstanceID), quote(name))

	list, _, err := sm.GetAllServiceBindings(ctx).FieldQuery(query).Execute()
	if err != nil {
		return BindingMatch{}, false, specifyAPIError(err)
	}

	items := list.GetItems()
	switch len(items) {
	case 0:
		return BindingMatch{}, false, nil
	case 1:
		return BindingMatch{ID: internal.Val(items[0].Id), CreatedAt: items[0].GetCreatedAt()}, true, nil
	default:
		return BindingMatch{}, false, errors.Errorf(
			"%d service bindings match name %q for service instance %q", len(items), name, serviceInstanceID)
	}
}

// AdoptablePair returns the external-name of the managed instance named
// instanceName and its binding bindingName, as used by ServiceManager and
// CloudManagement: "<instanceID>/<bindingID>", or the bare instance ID when
// the binding does not exist yet. found is false when no instance has that
// name on any plan.
//
// The instance is looked up by name across plans, then required to run on
// planID: an instance by that name on another plan is refused rather than
// treated as missing, since it is not the declared service and its name would
// make a create collide.
func AdoptablePair(ctx context.Context, l SemanticLookuper, planID, instanceName, bindingName string) (string, bool, error) {
	instance, found, err := l.FindServiceInstance(ctx, instanceName)
	if err != nil {
		return "", false, errors.Wrapf(err, "cannot look up service instance %q to adopt", instanceName)
	}
	if !found {
		return "", false, nil
	}
	if instance.PlanID != planID {
		return "", false, errPairPlanMismatch(instanceName, instance.ID, instance.PlanID, planID)
	}

	instanceID, bindingID, _, found, err := l.LookupInstanceAndBinding(ctx, planID, instanceName, bindingName)
	if err != nil {
		return "", false, errors.Wrapf(err, "cannot look up binding %q of service instance %q to adopt", bindingName, instanceName)
	}
	if !found {
		return "", false, nil
	}
	if bindingID == "" {
		return instanceID, true, nil
	}
	return instanceID + "/" + bindingID, true, nil
}

func errPairPlanMismatch(name, id, actualPlanID, declaredPlanID string) error {
	return errors.Errorf("refusing to adopt service instance %q (%s): it runs on service plan %s, the spec declares %s; "+
		"instance names are unique, so it can neither be adopted nor created under this name: "+
		"declare the plan it runs on to adopt it, or set another serviceInstanceName to create a new one",
		name, id, actualPlanID, declaredPlanID)
}

// quote renders s as a Service Manager query string literal. Service Manager
// escapes a single quote inside a literal by doubling it; names may contain
// one, and an unescaped quote makes the query fail to parse.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
