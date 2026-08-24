package v1alpha1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServiceBindingRotationRoundTrip guards issue #841: typed client writes must preserve rotation
// literals so the provider does not claim those fields and conflict with later server-side applies.
func TestServiceBindingRotationRoundTrip(t *testing.T) {
	in := []byte(`{
		"apiVersion": "account.btp.sap.crossplane.io/v1alpha1",
		"kind": "ServiceBinding",
		"metadata": {"name": "x"},
		"spec": {
			"forProvider": {"name": "x"},
			"rotation": {"frequency": "720h", "ttl": "1080h"}
		}
	}`)

	var cr ServiceBinding
	require.NoError(t, json.Unmarshal(in, &cr))

	require.NotNil(t, cr.Spec.Rotation)
	require.NotNil(t, cr.Spec.Rotation.Frequency)
	require.NotNil(t, cr.Spec.Rotation.TTL)

	out, err := json.Marshal(&cr)
	require.NoError(t, err)

	assert.Contains(t, string(out), `"frequency":"720h"`)
	assert.Contains(t, string(out), `"ttl":"1080h"`)
	assert.NotContains(t, string(out), "720h0m0s")
	assert.NotContains(t, string(out), "1080h0m0s")
}
