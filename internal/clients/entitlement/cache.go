package entitlement

import (
	"context"
	"fmt"
	"time"

	"github.com/maypok86/otter/v2"
	"github.com/maypok86/otter/v2/stats"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

var _ Client = (*CachingClient)(nil)

// CachingClient is a decorator around Client that caches DescribeInstance results
// and transparently invalidates on mutations.
type CachingClient struct {
	inner Client
	cache *otter.Cache[string, *Instance]
}

func NewCachingClient(inner Client, cache *otter.Cache[string, *Instance]) *CachingClient {
	return &CachingClient{inner: inner, cache: cache}
}

func (c *CachingClient) DescribeInstance(ctx context.Context, cr *v1alpha1.Entitlement) (*Instance, error) {
	return c.cache.Get(ctx, getCacheKey(cr), otter.LoaderFunc[string, *Instance](
		func(ctx context.Context, key string) (*Instance, error) {
			return c.inner.DescribeInstance(ctx, cr)
		},
	))
}

func (c *CachingClient) CreateInstance(ctx context.Context, cr *v1alpha1.Entitlement) error {
	err := c.inner.CreateInstance(ctx, cr)
	c.cache.Invalidate(getCacheKey(cr))
	return err
}

func (c *CachingClient) UpdateInstance(ctx context.Context, cr *v1alpha1.Entitlement) error {
	err := c.inner.UpdateInstance(ctx, cr)
	c.cache.Invalidate(getCacheKey(cr))
	return err
}

func (c *CachingClient) DeleteInstance(ctx context.Context, cr *v1alpha1.Entitlement) error {
	err := c.inner.DeleteInstance(ctx, cr)
	c.cache.Invalidate(getCacheKey(cr))
	return err
}

// NewInstanceCache creates a cache for entitlement instances with the given TTL.
// Stats recording is enabled for Prometheus metrics exposure.
func NewInstanceCache(ttl time.Duration) *otter.Cache[string, *Instance] {
	return otter.Must(&otter.Options[string, *Instance]{
		MaximumSize:      10_000,
		ExpiryCalculator: otter.ExpiryWriting[string, *Instance](ttl),
		StatsRecorder:    stats.NewCounter(),
	})
}

// RegisterCacheMetrics registers Prometheus metrics for entitlement cache statistics.
// Metrics are read from otter's stats on each Prometheus scrape — no background goroutine needed.
func RegisterCacheMetrics(cache *otter.Cache[string, *Instance], registry prometheus.Registerer) {
	registry.MustRegister(
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "crossplane_entitlement_cache_hits_total",
			Help: "Total number of entitlement cache hits.",
		}, func() float64 { return float64(cache.Stats().Hits) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "crossplane_entitlement_cache_misses_total",
			Help: "Total number of entitlement cache misses.",
		}, func() float64 { return float64(cache.Stats().Misses) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "crossplane_entitlement_cache_hit_ratio",
			Help: "Entitlement cache hit ratio (0.0-1.0).",
		}, func() float64 { return cache.Stats().HitRatio() }),
	)
}

func getCacheKey(cr *v1alpha1.Entitlement) string {
	return fmt.Sprintf("%s|%s|%s|%s",
		cr.GetProviderConfigReference().Name,
		cr.Spec.ForProvider.SubaccountGuid,
		cr.Spec.ForProvider.ServiceName,
		cr.Spec.ForProvider.ServicePlanName,
	)
}
