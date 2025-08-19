package environments

import (
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

type connDetailsAt struct {
	*managed.ConnectionDetails
	createdAt time.Time
}

// connDetailsMemoMap is a helper object to implement the memoization
// of the GetConnectionDetails method.
type connDetailsMemoMap map[string]*connDetailsAt

// get method returns from the memoization map a ConnectionDetails
// object if,
// - we have stored it earlier with the same instance label, and
// - the value is not expired
func (cdmm connDetailsMemoMap)get(instance *provisioningclient.BusinessEnvironmentInstanceResponseObject) (*managed.ConnectionDetails, bool) {
	if instance.Labels == nil {
		// If the instance has no Labels, we haven't stored
		// its connectection details before.
		return nil, false
	}
	cdAt, found := cdmm[*instance.Labels]
	if !found {
		// There is no ConnectionDetails in the hash
		return nil, false
	}
	if time.Since(cdAt.createdAt) > connDetailExpiry {
		// ConnectionDetails is expired, we have to ask for a
		// new value
		return nil, false
	}
	// Get the value from the hash
	return cdAt.ConnectionDetails, true
}

// set method stores a ConnectionDetails in the memoization map
func (cdmm connDetailsMemoMap)set(instance *provisioningclient.BusinessEnvironmentInstanceResponseObject, cd *managed.ConnectionDetails) {
	if instance.Labels == nil {
		// We can't store a ConnectionDetails if the instance has no Labels
		return
	}
	cdmm[*instance.Labels] = &connDetailsAt{
		ConnectionDetails: cd,
		createdAt: time.Now(),
	}
}

// invalidate method invalidates the cache item
func (cdmm connDetailsMemoMap)invalidate(instance *provisioningclient.BusinessEnvironmentInstanceResponseObject) {
	if instance.Labels == nil {
		// We can't invalidate a ConnectionDetails if the instance has no Labels
		return
	}
	delete(cdmm, *instance.Labels)
}

var connDetailExpiry = 5 * time.Minute // Cache expiration time
var connDetailsMemoizationMap connDetailsMemoMap = connDetailsMemoMap{}
