package internal

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
)

func TestVal(t *testing.T) {

	// nil pointer
	var ptrString *string
	assert.Equal(t, "", Val(ptrString))

	// value pointer
	str := "Foo"
	ptrString = &str
	assert.Equal(t, "Foo", Val(ptrString))

	// pointer to empty value
	emptyStr := ""
	ptrString = &emptyStr
	assert.Equal(t, "", Val(ptrString))

	// same tests for bool to ensure its generic
	var ptrBool *bool
	assert.Equal(t, false, Val(ptrBool))
	b := true
	ptrBool = &b
	assert.Equal(t, true, Val(ptrBool))

	emptyB := false
	ptrBool = &emptyB
	assert.Equal(t, false, Val(ptrBool))

}

func TestFlattenConnectionDetails(t *testing.T) {
	jsonBlob := []byte(`{"key1":"value1","key2":"value2"}`)
	mixedBlob := []byte(`{"nested_key":"nested_value"}`)

	cases := map[string]struct {
		reason string
		input  map[string][]byte
		want   map[string][]byte
		err    error
	}{
		"EmptyInput": {
			reason: "should handle empty input",
			input:  map[string][]byte{},
			want:   map[string][]byte{},
		},
		"NonJSONValue": {
			reason: "should keep non-JSON values as-is",
			input: map[string][]byte{
				"simple": []byte("value"),
			},
			want: map[string][]byte{
				"simple": []byte("value"),
			},
		},
		"JSONObjectValue": {
			reason: "should flatten JSON object values and preserve raw blob",
			input: map[string][]byte{
				"json_obj": jsonBlob,
			},
			want: map[string][]byte{
				"key1":                         []byte("value1"),
				"key2":                         []byte("value2"),
				providerv1alpha1.RawBindingKey: jsonBlob,
			},
		},
		"MixedValues": {
			reason: "should handle mixed JSON and non-JSON values",
			input: map[string][]byte{
				"simple":   []byte("simple_value"),
				"json_obj": mixedBlob,
			},
			want: map[string][]byte{
				"simple":                       []byte("simple_value"),
				"nested_key":                   []byte("nested_value"),
				providerv1alpha1.RawBindingKey: mixedBlob,
			},
		},
		"NestedObjectValue": {
			reason: "should re-marshal non-string nested values",
			input: map[string][]byte{
				"json_obj": []byte(`{"endpoints":{"api":"https://api.example.com"}}`),
			},
			want: map[string][]byte{
				"endpoints":                    []byte(`{"api":"https://api.example.com"}`),
				providerv1alpha1.RawBindingKey: []byte(`{"endpoints":{"api":"https://api.example.com"}}`),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := FlattenConnectionDetails(tc.input)

			if (err == nil) != (tc.err == nil) {
				t.Errorf("\n%s\nFlattenConnectionDetails(...): want err=%v, got err=%v\n", tc.reason, tc.err, err)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("\n%s\nFlattenConnectionDetails(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}
