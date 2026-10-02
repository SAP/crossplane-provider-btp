// Package externalstate exports the external (BTP-side) state of the managed
// resources this provider owns as a Prometheus gauge.
//
// A managed resource whose external counterpart has failed is only actionable
// if the failed population is discoverable. Conditions and events tell an
// operator about one resource at a time; this gauge makes "how many
// ServiceInstances are in state failed right now" a scrapeable number, which
// is what an alert can be written against.
package externalstate

import (
	"context"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
)

// maxStateLabelBytes caps the length of a state label value.
const maxStateLabelBytes = 64

// unknownState is the bucket every empty or unreadable external state falls
// into, so a missing observation never mints a series of its own.
const unknownState = "unknown"

// defaultListTimeout bounds a single List. Listing through the manager's cache
// starts an informer for that kind and waits for it to sync, and that wait only
// ends when the manager stops: an informer that can never sync - list/watch
// denied by RBAC, or a stalled initial LIST - would otherwise block collect()
// forever. The ticker would never fire again, the gauge would silently freeze
// at its last value, and the Runnable would not return on shutdown. With the
// bound, an unsyncable kind degrades into a skipped kind that is retried on the
// next tick.
const defaultListTimeout = 30 * time.Second

// gauge counts managed resources per managed kind and external state.
//
// Cardinality: kind x state only - never the resource name. BTP states come
// from a small closed vocabulary (succeeded / failed / in progress / OK /
// PROCESSING / PROCESSING_FAILED / STARTED / DELETING / ...); anything empty
// or unrecognised collapses into "unknown" and every value is length-capped,
// which keeps the series count in the low tens no matter how many resources
// the provider manages.
var gauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "btp_managed_resource_external_state",
	Help: "Number of managed resources per managed kind and external (atProvider) state.",
}, []string{"kind", "state"})

// Setup registers the gauge on controller-runtime's metrics registry - the one
// backing the provider's /metrics endpoint - and adds a Runnable that refreshes
// it from the manager's cache.
//
// Collection runs on a timer rather than at scrape time on purpose: a
// collector that lists at scrape time would make /metrics block on an unsynced
// informer, turning a metrics scrape into a liveness hazard.
//
// It reads through the manager's cache rather than issuing live LISTs: the
// controllers for these kinds run in the same process and already watch them,
// so the cache read is free, whereas an unpaginated LIST per kind on every
// tick would add exactly the kind of API-server load this provider has to stay
// clear of. Every List is bounded (see defaultListTimeout) so a kind whose
// informer cannot sync degrades to a skipped kind instead of wedging the
// collector.
func Setup(mgr ctrl.Manager, log logging.Logger, interval time.Duration) error {
	if interval <= 0 {
		return errors.Errorf("external-state metric interval must be positive, got %s", interval)
	}
	if err := metrics.Registry.Register(gauge); err != nil {
		// AlreadyRegisteredError is benign: Setup is idempotent so that tests
		// and any future second manager do not fail on a duplicate register.
		if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
			return err
		}
	}
	return mgr.Add(&collector{
		reader:   mgr.GetCache(),
		log:      log,
		interval: interval,
	})
}

// collector periodically refreshes the gauge from a cached reader.
type collector struct {
	reader   client.Reader
	log      logging.Logger
	interval time.Duration
	// listTimeout bounds a single List; zero means defaultListTimeout.
	listTimeout time.Duration
}

// Start implements manager.Runnable. It collects once immediately so the gauge
// is populated as soon as the cache is synced, then on every tick.
func (c *collector) Start(ctx context.Context) error {
	c.collect(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

// kindCollector names one managed kind and knows how to read the external
// state out of each of its instances.
type kindCollector struct {
	kind string
	list func() client.ObjectList
	// states returns one state string per item in the list.
	states func(client.ObjectList) []string
}

// kinds is the table of managed kinds whose external state is exported. It is
// deliberately small and explicit: only kinds that actually carry a
// platform-reported state are worth a series.
var kinds = []kindCollector{
	{
		kind: v1alpha1.ServiceInstanceKind,
		list: func() client.ObjectList { return &v1alpha1.ServiceInstanceList{} },
		states: func(l client.ObjectList) []string {
			items := l.(*v1alpha1.ServiceInstanceList).Items
			out := make([]string, 0, len(items))
			for i := range items {
				out = append(out, items[i].Status.AtProvider.State)
			}
			return out
		},
	},
	{
		kind: v1alpha1.EntitlementKind,
		list: func() client.ObjectList { return &v1alpha1.EntitlementList{} },
		states: func(l client.ObjectList) []string {
			items := l.(*v1alpha1.EntitlementList).Items
			out := make([]string, 0, len(items))
			for i := range items {
				at := items[i].Status.AtProvider
				if at == nil || at.Assigned == nil {
					out = append(out, "")
					continue
				}
				out = append(out, at.Assigned.EntityState)
			}
			return out
		},
	},
	{
		kind: v1alpha1.SubaccountKind,
		list: func() client.ObjectList { return &v1alpha1.SubaccountList{} },
		states: func(l client.ObjectList) []string {
			items := l.(*v1alpha1.SubaccountList).Items
			out := make([]string, 0, len(items))
			for i := range items {
				out = append(out, internal.Val(items[i].Status.AtProvider.Status))
			}
			return out
		},
	},
}

// collect rebuilds the gauge one kind at a time. A kind's series are replaced
// (DeletePartialMatch, then Set) only after its List succeeded, so states that
// no longer have any resource are dropped without a "failed" series lingering —
// while a kind whose List failed this tick keeps its last-known series instead
// of blinking to no-data, which would trip absence-based alerting.
//
// A List error is logged and that kind is skipped: a trimmed installation may
// have the CRD of a disabled controller removed entirely, and a metrics
// collector must never take the provider down over it. Each List is bounded so
// a kind whose informer cannot sync is skipped rather than hanging the
// collector (see defaultListTimeout).
func (c *collector) collect(ctx context.Context) {
	for _, k := range kinds {
		list := k.list()
		if err := c.listKind(ctx, list); err != nil {
			c.log.Debug("cannot list managed resources for the external-state metric",
				"kind", k.kind, "error", err)
			continue
		}
		perState := map[string]int{}
		for _, s := range k.states(list) {
			perState[normalizeState(s)]++
		}
		gauge.DeletePartialMatch(prometheus.Labels{"kind": k.kind})
		for state, n := range perState {
			gauge.WithLabelValues(k.kind, state).Set(float64(n))
		}
	}
}

// listKind performs one bounded List. The timeout is what keeps an unsyncable
// informer from wedging the whole collector.
func (c *collector) listKind(ctx context.Context, list client.ObjectList) error {
	timeout := c.listTimeout
	if timeout <= 0 {
		timeout = defaultListTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.reader.List(ctx, list)
}

// normalizeState maps a platform-reported state onto a bounded label value:
// an unobserved (empty) state becomes "unknown", and an unexpectedly long
// value is capped rather than dropped, so a new legitimate BTP state still
// shows up instead of being silently hidden.
func normalizeState(s string) string {
	if s == "" {
		return unknownState
	}
	if len(s) > maxStateLabelBytes {
		return s[:maxStateLabelBytes]
	}
	return s
}
