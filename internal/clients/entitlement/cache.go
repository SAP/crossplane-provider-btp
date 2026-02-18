package entitlement

import (
	"time"

	"github.com/maypok86/otter/v2"
)

// NewInstanceCache creates a cache for entitlement instances with a fixed 10s TTL.
// Mutations (Create/Update/Delete) explicitly invalidate the cache via ClearDescribeInstanceCache.
func NewInstanceCache() *otter.Cache[string, *Instance] {
	return otter.Must(&otter.Options[string, *Instance]{
		MaximumSize:      10_000,
		ExpiryCalculator: otter.ExpiryWriting[string, *Instance](10 * time.Second),
	})
}
