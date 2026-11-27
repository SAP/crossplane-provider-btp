package externaloperation

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/require"
)

const (
	publicKind = "Subaccount"
	metric     = "provider_btp_external_operation_duration_seconds"
)

func newTestRecorder(t *testing.T) (*Recorder, *prometheus.Registry, *time.Time) {
	t.Helper()
	reg := prometheus.NewPedanticRegistry()
	r, err := NewRecorder(reg)
	require.NoError(t, err)
	now := time.Unix(0, 0)
	r.now = func() time.Time { return now }
	return r, reg, &now
}

type sample struct {
	operation, outcome string
	elapsed            time.Duration
}

func requireSamples(t *testing.T, reg *prometheus.Registry, want ...sample) {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	require.Len(t, families, 1)
	require.Equal(t, metric, families[0].GetName())
	series := families[0].GetMetric()
	require.Len(t, series, len(want))
	byLabels := make(map[[2]string]time.Duration, len(series))
	for _, s := range series {
		labels := make(map[string]string)
		for _, label := range s.GetLabel() {
			labels[label.GetName()] = label.GetValue()
		}
		require.Len(t, labels, 3)
		require.Equal(t, publicKind, labels["kind"])
		histogram := s.GetHistogram()
		require.NotNil(t, histogram)
		require.Equal(t, uint64(1), histogram.GetSampleCount(), "labels: %v", labels)
		byLabels[[2]string{labels["operation"], labels["outcome"]}] = time.Duration(histogram.GetSampleSum() * float64(time.Second))
	}
	for _, s := range want {
		elapsed, ok := byLabels[[2]string{s.operation, s.outcome}]
		require.True(t, ok, "missing %s/%s", s.operation, s.outcome)
		require.Equal(t, s.elapsed, elapsed, "duration for %s/%s", s.operation, s.outcome)
	}
}

func TestConnect(t *testing.T) {
	failure := errors.New("fake connect failure")
	client := &managed.NopClient{}
	cases := []struct {
		name    string
		client  managed.ExternalClient
		err     error
		outcome string
	}{
		{name: "success", client: client, outcome: "success"},
		{name: "success with nil client", outcome: "success"},
		{name: "error with nil client", err: failure, outcome: "error"},
		{name: "error with non-nil client", client: client, err: failure, outcome: "error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, reg, now := newTestRecorder(t)
			ctx, mg := t.Context(), &fake.LegacyManaged{}
			const elapsed = 250 * time.Millisecond
			c := managed.ExternalConnectorFn(func(gotCtx context.Context, gotMG resource.Managed) (managed.ExternalClient, error) {
				require.True(t, gotCtx == ctx)
				require.Same(t, mg, gotMG)
				*now = now.Add(elapsed)
				return tc.client, tc.err
			})
			got, err := r.Wrap(publicKind, c).Connect(ctx, mg)
			require.True(t, err == tc.err, "error = %v, want exact error %v", err, tc.err)
			if tc.err == nil && tc.client != nil {
				wrapped, ok := got.(*externalClient)
				require.True(t, ok, "client type = %T, want a wrapped client", got)
				require.Same(t, tc.client, wrapped.ExternalClient)
			} else {
				require.True(t, got == tc.client, "client = %v, want original return %v", got, tc.client)
			}
			requireSamples(t, reg, sample{"connect", tc.outcome, elapsed})
		})
	}
}

func TestClientOperations(t *testing.T) {
	details := managed.ConnectionDetails{"key": []byte("fake connection detail")}
	observe := managed.ExternalObservation{ResourceExists: true, ResourceLateInitialized: true, ConnectionDetails: details}
	create := managed.ExternalCreation{ConnectionDetails: details}
	update := managed.ExternalUpdate{ConnectionDetails: details}
	deleteResult := managed.ExternalDelete{AdditionalDetails: managed.AdditionalDetails{"state": "pending"}}
	type operation struct {
		name string
		want any
		call func(managed.ExternalClient, context.Context, resource.Managed) (any, error)
	}
	operations := []operation{
		{"observe", observe, func(c managed.ExternalClient, ctx context.Context, mg resource.Managed) (any, error) {
			return c.Observe(ctx, mg)
		}},
		{"create", create, func(c managed.ExternalClient, ctx context.Context, mg resource.Managed) (any, error) {
			return c.Create(ctx, mg)
		}},
		{"update", update, func(c managed.ExternalClient, ctx context.Context, mg resource.Managed) (any, error) {
			return c.Update(ctx, mg)
		}},
		{"delete", deleteResult, func(c managed.ExternalClient, ctx context.Context, mg resource.Managed) (any, error) {
			return c.Delete(ctx, mg)
		}},
	}
	outcomes := []struct {
		name string
		err  error
	}{
		{"success", nil},
		{"error", errors.New("fake operation failure")},
	}
	for _, op := range operations {
		for _, outcome := range outcomes {
			t.Run(op.name+"/"+outcome.name, func(t *testing.T) {
				r, reg, now := newTestRecorder(t)
				ctx, mg := t.Context(), &fake.LegacyManaged{}
				const elapsed = 500 * time.Millisecond
				calls := 0
				advance := func(gotCtx context.Context, gotMG resource.Managed) {
					require.True(t, gotCtx == ctx)
					require.Same(t, mg, gotMG)
					calls++
					*now = now.Add(elapsed)
				}
				disconnectErr := errors.New("fake disconnect failure")
				var disconnects int
				inner := managed.ExternalClientFns{
					ObserveFn: func(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
						advance(ctx, mg)
						return observe, outcome.err
					},
					CreateFn: func(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
						advance(ctx, mg)
						return create, outcome.err
					},
					UpdateFn: func(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
						advance(ctx, mg)
						return update, outcome.err
					},
					DeleteFn: func(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
						advance(ctx, mg)
						return deleteResult, outcome.err
					},
					DisconnectFn: func(gotCtx context.Context) error {
						require.True(t, gotCtx == ctx)
						disconnects++
						return disconnectErr
					},
				}
				connector := managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) { return inner, nil })
				client, err := r.Wrap(publicKind, connector).Connect(ctx, mg)
				require.NoError(t, err)
				got, err := op.call(client, ctx, mg)
				require.Equal(t, op.want, got)
				require.True(t, err == outcome.err, "error = %v, want exact error %v", err, outcome.err)
				require.Equal(t, 1, calls)
				require.True(t, client.Disconnect(ctx) == disconnectErr)
				require.Equal(t, 1, disconnects)
				requireSamples(t, reg, sample{"connect", "success", 0}, sample{op.name, outcome.name, elapsed})
			})
		}
	}
}

func TestDuplicateRegistration(t *testing.T) {
	_, reg, _ := newTestRecorder(t)
	r, err := NewRecorder(reg)
	require.Nil(t, r)
	var duplicate prometheus.AlreadyRegisteredError
	require.ErrorAs(t, err, &duplicate)
}

func TestMetricsEndpoint(t *testing.T) {
	r, reg, now := newTestRecorder(t)
	connector := managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) {
		*now = now.Add(250 * time.Millisecond)
		return nil, errors.New("fake connect failure")
	})
	_, err := r.Wrap(publicKind, connector).Connect(t.Context(), &fake.LegacyManaged{})
	require.Error(t, err)

	// controller-runtime mounts promhttp.HandlerFor on /metrics. Use the same
	// arrangement with an isolated registry, without starting a manager.
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	require.Contains(t, body, "# TYPE "+metric+" histogram\n")
	const labels = `kind="Subaccount",operation="connect",outcome="error"`
	require.Contains(t, body, metric+`_bucket{`+labels+`,le="0.1"} 0`+"\n")
	require.Contains(t, body, metric+`_bucket{`+labels+`,le="0.25"} 1`+"\n")
	require.Contains(t, body, metric+`_sum{`+labels+`} 0.25`+"\n")
	require.Contains(t, body, metric+`_count{`+labels+`} 1`+"\n")
}
