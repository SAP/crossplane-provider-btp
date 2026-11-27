// Package externaloperation measures calls at the managed reconciler's external boundary.
package externaloperation

import (
	"context"
	"fmt"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/prometheus/client_golang/prometheus"
)

const metricName = "provider_btp_external_operation_duration_seconds"

// Recorder records external operation durations for multiple managed resource kinds.
type Recorder struct {
	duration *prometheus.HistogramVec
	now      func() time.Time
}

// NewRecorder registers the histogram. A registration conflict is returned as
// an error; callers should construct one recorder and share it across controllers.
func NewRecorder(reg prometheus.Registerer) (*Recorder, error) {
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: metricName,
		Help: "Duration of managed external operations from method entry to return in seconds.",
		// Include both fast local calls and operations that spend seconds or
		// minutes waiting on remote APIs.
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
	}, []string{"kind", "operation", "outcome"})
	if err := reg.Register(duration); err != nil {
		return nil, fmt.Errorf("register %s: %w", metricName, err)
	}
	return &Recorder{duration: duration, now: time.Now}, nil
}

// Wrap instruments a reconciler's external connector and its clients. kind
// must be the public Kubernetes Kind from the controller's registration, not
// derived from individual resources. Internal delegate connectors are not wrapped.
func (r *Recorder) Wrap(kind string, connector managed.ExternalConnector) managed.ExternalConnector {
	return managed.ExternalConnectorFn(func(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
		start := r.now()
		client, err := connector.Connect(ctx, mg)
		defer r.observe(start, kind, "connect", err)
		if err != nil || client == nil {
			return client, err
		}
		return &externalClient{ExternalClient: client, recorder: r, kind: kind}, nil
	})
}

func (r *Recorder) observe(start time.Time, kind, operation string, err error) {
	elapsed := r.now().Sub(start).Seconds()
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	r.duration.WithLabelValues(kind, operation, outcome).Observe(elapsed)
}

// Embed the client to forward Disconnect unchanged and without recording it.
type externalClient struct {
	managed.ExternalClient
	recorder *Recorder
	kind     string
}

func (c *externalClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	start := c.recorder.now()
	result, err := c.ExternalClient.Observe(ctx, mg)
	defer c.recorder.observe(start, c.kind, "observe", err)
	return result, err
}

func (c *externalClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	start := c.recorder.now()
	result, err := c.ExternalClient.Create(ctx, mg)
	defer c.recorder.observe(start, c.kind, "create", err)
	return result, err
}

func (c *externalClient) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	start := c.recorder.now()
	result, err := c.ExternalClient.Update(ctx, mg)
	defer c.recorder.observe(start, c.kind, "update", err)
	return result, err
}

func (c *externalClient) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	start := c.recorder.now()
	result, err := c.ExternalClient.Delete(ctx, mg)
	defer c.recorder.observe(start, c.kind, "delete", err)
	return result, err
}
