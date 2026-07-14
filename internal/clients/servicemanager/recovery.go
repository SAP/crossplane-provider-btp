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

// SemanticLookuper adopts a BTP resource that already exists in the external
// system but is not yet linked to its Kubernetes CR via the
// crossplane.io/external-name annotation.
//
// It is used by the "orphaned external-name adoption" heal path. The heal path
// runs when a managed resource carries only a fallback external-name (empty or
// equal to metadata.name) yet BTP already has the corresponding resource.
//
// All lookups are performed against the BTP Service Manager API using
// credentials that are already scoped to a single subaccount. The subaccount
// component of a semantic key is therefore enforced implicitly by the
// credential scope, so the field queries below only need to disambiguate
// within that subaccount.
//
// Every lookup also returns the BTP-reported created_at timestamp of the
// matched resource, so the caller can enforce an ownership check
// (recovery.IsOwnedByCR) before actually adopting: only resources whose
// created_at is at (or after) the CR's own creationTimestamp were plausibly
// created by our Create() call — anything older is a brownfield resource
// and must be adopted explicitly via the ADR-documented import flow.
type SemanticLookuper interface {
	// LookupServiceInstance returns the BTP GUID and created_at of the service
	// instance whose name matches within the credential's subaccount.
	// found is false when no instance matches. An error is returned when more
	// than one instance matches (refuse to guess) or the API call fails.
	LookupServiceInstance(ctx context.Context, name string) (guid string, createdAt time.Time, found bool, err error)

	// LookupServiceBinding returns the BTP GUID and created_at of the service
	// binding matching (serviceInstanceID, name). When no binding matches name
	// exactly (e.g. the binding was created with a rotation suffix
	// "<name>-<random>"), it falls back to the single ready binding whose name
	// has the "<name>-" prefix. found is false when nothing matches. An error
	// is returned when the match is ambiguous or the API call fails.
	LookupServiceBinding(ctx context.Context, serviceInstanceID, name string) (guid string, createdAt time.Time, found bool, err error)

	// LookupInstanceAndBinding returns the (serviceInstanceID, serviceBindingID)
	// pair for the instance matching (planID, instanceName) within the
	// credential's subaccount, together with the binding matching bindingName
	// and the INSTANCE's created_at. Keying on the instance name (not just the
	// plan) is required because the subaccount-admin plan can hold both the
	// managed instance and an access instance; keying the binding on
	// bindingName is required so a transient admin binding (minted for
	// visibility) is never returned. serviceBindingID is empty when the
	// instance exists but the named binding does not. found is false when no
	// instance matches. An error is returned when more than one instance
	// matches or an API call fails.
	//
	// The returned created_at is the INSTANCE's — the two-phase Create writes
	// the instance first, so instance-createdAt is the earliest signal of
	// "we created this resource" and the correct reference for the ownership
	// check upstream.
	LookupInstanceAndBinding(ctx context.Context, planID, instanceName, bindingName string) (serviceInstanceID, serviceBindingID string, instanceCreatedAt time.Time, found bool, err error)
}

// compile-time assertion that the SM API client satisfies the lookup contract.
var _ SemanticLookuper = &ServiceManagerClient{}

// LookupServiceInstance implements SemanticLookuper.
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
			"refusing to adopt: %d service instances match name %q in this subaccount", len(items), name)
	}
}

// LookupServiceBinding implements SemanticLookuper.
func (sm *ServiceManagerClient) LookupServiceBinding(ctx context.Context, serviceInstanceID, name string) (string, time.Time, bool, error) {
	// exact match first
	query := fmt.Sprintf("service_instance_id eq '%s' and name eq '%s'", serviceInstanceID, name)
	list, _, err := sm.GetAllServiceBindings(ctx).FieldQuery(query).Execute()
	if err != nil {
		return "", time.Time{}, false, specifyAPIError(err)
	}
	items := list.GetItems()
	switch len(items) {
	case 0:
		// no exact match: fall back to rotation naming "<name>-<random>".
		return sm.lookupRotatedBinding(ctx, serviceInstanceID, name)
	case 1:
		return internal.Val(items[0].Id), items[0].GetCreatedAt(), true, nil
	default:
		return "", time.Time{}, false, errors.Errorf(
			"refusing to adopt: %d service bindings match name %q for service instance %q",
			len(items), name, serviceInstanceID)
	}
}

// lookupRotatedBinding handles rotated bindings, whose names are
// "<name>-<random>" (see servicebinding.GenerateRandomName). It adopts the
// single READY binding carrying that prefix; if there is not exactly one ready
// candidate it declines (found=false, or an error when ambiguous) rather than
// guessing which rotated key is authoritative.
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
			"refusing to adopt: %d ready rotated bindings match prefix %q for service instance %q",
			len(ready), prefix, serviceInstanceID)
	}
}

// LookupInstanceAndBinding implements SemanticLookuper.
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
			"refusing to adopt: %d service instances match plan %q name %q in this subaccount",
			len(instanceItems), planID, instanceName)
	}

	instanceID := internal.Val(instanceItems[0].Id)
	instanceCreatedAt := instanceItems[0].GetCreatedAt()

	// Look up the paired managed binding BY NAME. Filtering by name is required
	// so a transient subaccount-admin binding (minted only for visibility and
	// then deleted) is never adopted. Absence of the binding is not an error:
	// the instance may exist while its managed binding still needs to be created.
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
			"refusing to adopt: %d bindings match name %q for service instance %q",
			len(bindingItems), bindingName, instanceID)
	}
}
