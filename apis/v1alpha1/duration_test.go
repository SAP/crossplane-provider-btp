package v1alpha1

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestDurationRoundTrip guards the regression described in issue #841.
func TestDurationRoundTrip(t *testing.T) {
	for _, literal := range []string{"720h", "1080h", "3m", "1h15m", "90m", "0h", "500ms", "720h0m0s"} {
		t.Run(literal, func(t *testing.T) {
			in := []byte(`"` + literal + `"`)

			var d Duration
			assert.NoError(t, json.Unmarshal(in, &d))

			out, err := json.Marshal(d)
			assert.NoError(t, err)
			assert.Equal(t, string(in), string(out))
		})
	}
}

func TestDurationUnmarshalSetsValue(t *testing.T) {
	tests := map[string]time.Duration{
		"720h": 720 * time.Hour,
		"90m":  90 * time.Minute,
	}
	for literal, want := range tests {
		t.Run(literal, func(t *testing.T) {
			var d Duration
			assert.NoError(t, json.Unmarshal([]byte(`"`+literal+`"`), &d))
			assert.Equal(t, want, d.Duration)
		})
	}
}

func TestDurationMarshalGoConstructed(t *testing.T) {
	out, err := json.Marshal(Duration{Duration: time.Hour})
	assert.NoError(t, err)
	assert.JSONEq(t, `"1h0m0s"`, string(out))
}

// TestDurationMarshalAfterMutation pins the invariant that the preserved literal only describes the
// duration it was decoded from: assigning a new value must fall back to the canonical form.
func TestDurationMarshalAfterMutation(t *testing.T) {
	var d Duration
	assert.NoError(t, json.Unmarshal([]byte(`"720h"`), &d))

	d.Duration = time.Hour

	out, err := json.Marshal(d)
	assert.NoError(t, err)
	assert.JSONEq(t, `"1h0m0s"`, string(out))
	assert.Equal(t, "1h0m0s", d.String())
	assert.Equal(t, "1h0m0s", d.ToUnstructured())
}

func TestDurationUnmarshalErrors(t *testing.T) {
	for _, in := range []string{`"nonsense"`, `"720"`, `123`} {
		t.Run(in, func(t *testing.T) {
			var d Duration
			assert.Error(t, json.Unmarshal([]byte(in), &d))
		})
	}
}

func TestDurationDeepCopyPreservesLiteral(t *testing.T) {
	var d Duration
	assert.NoError(t, json.Unmarshal([]byte(`"720h"`), &d))

	out, err := json.Marshal(d.DeepCopy())
	assert.NoError(t, err)
	assert.JSONEq(t, `"720h"`, string(out))
}

func TestDurationToUnstructuredPreservesLiteral(t *testing.T) {
	var d Duration
	assert.NoError(t, json.Unmarshal([]byte(`"720h"`), &d))
	assert.Equal(t, "720h", d.ToUnstructured())
}

func TestDurationEqualIgnoresLiteral(t *testing.T) {
	var a, b Duration
	assert.NoError(t, json.Unmarshal([]byte(`"90m"`), &a))
	assert.NoError(t, json.Unmarshal([]byte(`"1h30m0s"`), &b))

	assert.True(t, a.Equal(b))
	assert.NotEqual(t, a.String(), b.String(), "the literals must still differ, otherwise the round trip is not preserved")
}
