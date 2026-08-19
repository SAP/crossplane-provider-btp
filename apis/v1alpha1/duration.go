package v1alpha1

import (
	"encoding/json"
	"time"
)

// Duration preserves its original JSON string representation across round trips.
//
// Unlike metav1.Duration, it does not canonicalize values such as "720h" to "720h0m0s",
// avoiding the managed-fields conflicts reported in issue #841. Issue #892 specifies the
// fields that use this type.
//
// +kubebuilder:validation:Type=string
type Duration struct {
	time.Duration

	// text is the literal Duration was decoded from, and parsed is the value it decoded to. The
	// literal only describes parsed, so a caller that assigns Duration gets the canonical form
	// instead. Both are zero for values constructed in Go.
	text   string
	parsed time.Duration
}

// MarshalJSON implements json.Marshaler.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = parsed
	d.parsed = parsed
	d.text = s
	return nil
}

// ToUnstructured preserves the original literal during structured-to-unstructured conversion.
func (d Duration) ToUnstructured() interface{} {
	return d.String()
}

// Equal compares elapsed time and ignores the preserved literal.
// go-cmp uses this method because Duration contains unexported state.
func (d Duration) Equal(o Duration) bool { return d.Duration == o.Duration }

// String shadows the promoted time.Duration.String so logging and serialization agree.
func (d Duration) String() string {
	if d.text != "" && d.Duration == d.parsed {
		return d.text
	}
	return d.Duration.String()
}
