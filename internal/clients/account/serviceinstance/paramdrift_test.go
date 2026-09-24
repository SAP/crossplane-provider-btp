package serviceinstanceclient

import (
	"testing"
)

// m is a shorthand for map[string]interface{}
func m(pairs ...interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out[pairs[i].(string)] = pairs[i+1]
	}
	return out
}

// sl is a shorthand for []interface{}
func sl(elems ...interface{}) []interface{} {
	return elems
}

func TestParameterDrift(t *testing.T) {
	cases := map[string]struct {
		desired     map[string]interface{}
		lastApplied map[string]interface{}
		observed    map[string]interface{}
		wantDrift   bool
	}{
		// --- no drift cases ---

		"all empty": {
			desired: nil, lastApplied: nil, observed: nil,
			wantDrift: false,
		},
		"desired==observed, no last-applied": {
			desired:     m("foo", "bar"),
			lastApplied: nil,
			observed:    m("foo", "bar"),
			wantDrift:   false,
		},
		"desired==observed==lastApplied": {
			desired:     m("foo", "bar"),
			lastApplied: m("foo", "bar"),
			observed:    m("foo", "bar"),
			wantDrift:   false,
		},
		"BTP extra key (absent in desired and last-applied)": {
			desired:     m("foo", "bar"),
			lastApplied: m("foo", "bar"),
			observed:    m("foo", "bar", "btp_extra", "value"),
			wantDrift:   false,
		},
		"desired absent, last-applied absent, observed present = BTP extra": {
			desired:     m("foo", "bar"),
			lastApplied: m("foo", "bar"),
			observed:    m("foo", "bar", "injected_by_btp", 42.0),
			wantDrift:   false,
		},
		"key already removed from BTP (desired absent, last-applied present, observed absent)": {
			desired:     m("foo", "bar"),
			lastApplied: m("foo", "bar", "old", "val"),
			observed:    m("foo", "bar"),
			wantDrift:   false,
		},
		"nested object no drift": {
			desired:     m("cfg", m("a", 1.0, "b", 2.0)),
			lastApplied: m("cfg", m("a", 1.0, "b", 2.0)),
			observed:    m("cfg", m("a", 1.0, "b", 2.0)),
			wantDrift:   false,
		},
		"array order-invariant equal (reversed)": {
			desired:     m("tags", sl("c", "a", "b")),
			lastApplied: nil,
			observed:    m("tags", sl("b", "c", "a")),
			wantDrift:   false,
		},
		"array identical": {
			desired:     m("tags", sl("x", "y")),
			lastApplied: nil,
			observed:    m("tags", sl("x", "y")),
			wantDrift:   false,
		},
		"number type preserved": {
			desired:     m("count", 42.0),
			lastApplied: m("count", 42.0),
			observed:    m("count", 42.0),
			wantDrift:   false,
		},
		"nested removal already converged": {
			desired:     m("cfg", m("a", 1.0)),
			lastApplied: m("cfg", m("a", 1.0, "b", 2.0)),
			observed:    m("cfg", m("a", 1.0)),
			wantDrift:   false,
		},

		// --- drift cases ---

		"value changed in spec": {
			desired:     m("foo", "new"),
			lastApplied: m("foo", "old"),
			observed:    m("foo", "old"),
			wantDrift:   true,
		},
		"new top-level key added to spec, not yet in BTP": {
			desired:     m("foo", "bar", "added", "value"),
			lastApplied: m("foo", "bar"),
			observed:    m("foo", "bar"),
			wantDrift:   true,
		},
		"new nested key added to spec, not yet in BTP": {
			desired:     m("cfg", m("a", 1.0, "added", "value")),
			lastApplied: m("cfg", m("a", 1.0)),
			observed:    m("cfg", m("a", 1.0)),
			wantDrift:   true,
		},
		"spec key never applied and absent in BTP": {
			desired:     m("dashboard", m("custom_label", "test")),
			lastApplied: nil,
			observed:    m(),
			wantDrift:   true,
		},
		"BTP value mutated (desired==last-applied != observed)": {
			desired:     m("foo", "A"),
			lastApplied: m("foo", "A"),
			observed:    m("foo", "mutated"),
			wantDrift:   true,
		},
		"key removed from spec (user removal)": {
			desired:     m("foo", "bar"),
			lastApplied: m("foo", "bar", "old", "val"),
			observed:    m("foo", "bar", "old", "val"),
			wantDrift:   true,
		},
		"nested object value changed": {
			desired:     m("cfg", m("a", 1.0, "b", 99.0)),
			lastApplied: m("cfg", m("a", 1.0, "b", 2.0)),
			observed:    m("cfg", m("a", 1.0, "b", 2.0)),
			wantDrift:   true,
		},
		"nested key removed": {
			desired:     m("cfg", m("a", 1.0)),
			lastApplied: m("cfg", m("a", 1.0, "b", 2.0)),
			observed:    m("cfg", m("a", 1.0, "b", 2.0)),
			wantDrift:   true,
		},
		"array different element": {
			desired:     m("tags", sl("x", "z")),
			lastApplied: nil,
			observed:    m("tags", sl("x", "y")),
			wantDrift:   true,
		},
		"array different length": {
			desired:     m("tags", sl("x")),
			lastApplied: nil,
			observed:    m("tags", sl("x", "y")),
			wantDrift:   true,
		},
		"array multiset: duplicate counts differ": {
			desired:     m("tags", sl("a", "a", "b")),
			lastApplied: nil,
			observed:    m("tags", sl("a", "b", "b")),
			wantDrift:   true,
		},
		"shape mismatch: desired map, observed scalar": {
			desired:     m("cfg", m("a", 1.0)),
			lastApplied: nil,
			observed:    m("cfg", "scalar"),
			wantDrift:   true,
		},
		"shape mismatch: desired scalar, observed map": {
			desired:     m("cfg", "scalar"),
			lastApplied: nil,
			observed:    m("cfg", m("a", 1.0)),
			wantDrift:   true,
		},
		"string true != bool true": {
			desired:     m("flag", "true"),
			lastApplied: m("flag", "true"),
			observed:    m("flag", true),
			wantDrift:   true,
		},
		"number 42 != string 42": {
			desired:     m("n", "42"),
			lastApplied: m("n", "42"),
			observed:    m("n", 42.0),
			wantDrift:   true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, diff := ParameterDrift(tc.desired, tc.lastApplied, tc.observed)
			if got != tc.wantDrift {
				t.Errorf("ParameterDrift() drift=%v want=%v (diff=%q)", got, tc.wantDrift, diff)
			}
			if tc.wantDrift && diff == "" {
				t.Error("ParameterDrift() returned drift=true but empty diff string")
			}
			if !tc.wantDrift && diff != "" {
				t.Errorf("ParameterDrift() returned drift=false but non-empty diff=%q", diff)
			}
		})
	}
}

func TestCanonicalParameterJSON(t *testing.T) {
	cases := map[string]struct {
		in   map[string]interface{}
		want string
	}{
		"nil":   {nil, ""},
		"empty": {m(), ""},
		"single key": {m("a", "b"), `{"a":"b"}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := CanonicalParameterJSON(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeParameterJSON(t *testing.T) {
	cases := map[string]struct {
		in      string
		wantNil bool
		wantKey string
		wantVal interface{}
	}{
		"empty string": {in: "", wantNil: true},
		"valid json":   {in: `{"a":"b"}`, wantKey: "a", wantVal: "b"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := DecodeParameterJSON(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNil && got != nil {
				t.Errorf("expected nil, got %v", got)
			}
			if !tc.wantNil {
				if v := got[tc.wantKey]; v != tc.wantVal {
					t.Errorf("got[%q]=%v want %v", tc.wantKey, v, tc.wantVal)
				}
			}
		})
	}
}
