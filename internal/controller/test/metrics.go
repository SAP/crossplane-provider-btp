package test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// ExternalOperation identifies an expected histogram series with one sample.
type ExternalOperation struct {
	Kind, Operation, Outcome string
}

// RequireExternalOperations checks all series and sample counts in an isolated registry.
func RequireExternalOperations(t *testing.T, reg *prometheus.Registry, want ...ExternalOperation) {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	if len(want) == 0 {
		require.Empty(t, families)
		return
	}
	require.Len(t, families, 1)
	require.Equal(t, "provider_btp_external_operation_duration_seconds", families[0].GetName())
	var got []ExternalOperation
	for _, metric := range families[0].GetMetric() {
		require.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
		labels := make(map[string]string)
		for _, label := range metric.GetLabel() {
			labels[label.GetName()] = label.GetValue()
		}
		require.Len(t, labels, 3)
		got = append(got, ExternalOperation{labels["kind"], labels["operation"], labels["outcome"]})
	}
	require.ElementsMatch(t, want, got)
}
