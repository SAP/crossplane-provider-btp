package environments

import (
	"testing"
)

// kymaAzureSchema is a hand-built representation of the Kyma azure
// updateSchema, sufficient for exercising every rule the diff helper
// implements. Its shape must stay in sync with what parseSchema produces
// against kymaAzureUpdateSchema in schema_test.go — if they drift, both
// should be updated together.
func kymaAzureSchemaForDiffTest() *Schema {
	return &Schema{Properties: map[string]Property{
		"administrators":   {Type: "array"},
		"autoScalerMax":    {Type: "integer", Default: float64(20)},
		"autoScalerMin":    {Type: "integer", Default: float64(3)},
		"ingressFiltering": {Type: "boolean", Default: false},
		"machineType":      {Type: "string"},
		"name":             {Type: "string"},
		"gvisor": {
			Type: "object",
			Properties: map[string]Property{
				"enabled": {Type: "boolean", Default: false},
			},
		},
		"accessControlList": {
			Type: "object",
			Properties: map[string]Property{
				"allowedCIDRs": {Type: "array"},
			},
		},
	}}
}

// baseDesired mirrors a valid, healthy KymaEnvironment spec (with the
// provider-injected name default already applied).
func baseDesired() map[string]any {
	return map[string]any{
		"administrators": []any{"justin.luong@sap.com"},
		"autoScalerMax":  float64(3),
		"autoScalerMin":  float64(3),
		"machineType":    "Standard_D4_v3",
		"name":           "my-kyma-environment",
		"region":         "westeurope", // create-only; must not manifest as drift
	}
}

func TestDiffAgainstUpdateSchema(t *testing.T) {
	schema := kymaAzureSchemaForDiffTest()

	tests := map[string]struct {
		desired         map[string]any
		current         map[string]any
		wantNeedsUpdate bool
	}{
		"NoDrift_ExactMatch": {
			desired:         baseDesired(),
			current:         baseDesired(),
			wantNeedsUpdate: false,
		},

		// The issue #682 repro: BTP materialised the two defaulted fields
		// into current, spec doesn't have them. Must NOT be flagged as drift.
		"BugRepro_DefaultsMaterialisedInCurrent": {
			desired: baseDesired(),
			current: func() map[string]any {
				c := baseDesired()
				c["ingressFiltering"] = false
				c["gvisor"] = map[string]any{"enabled": false}
				return c
			}(),
			wantNeedsUpdate: false,
		},

		// Reporter's workaround: same three keys explicitly in desired.
		// Must remain non-drifting.
		"ReporterWorkaround_SchemaDefaultsInDesired": {
			desired: func() map[string]any {
				d := baseDesired()
				d["ingressFiltering"] = false
				d["gvisor"] = map[string]any{"enabled": false}
				return d
			}(),
			current: func() map[string]any {
				c := baseDesired()
				c["ingressFiltering"] = false
				c["gvisor"] = map[string]any{"enabled": false}
				return c
			}(),
			wantNeedsUpdate: false,
		},

		// A non-default observed value on a schema-defaulted field IS real
		// drift — someone flipped ingressFiltering to true out-of-band.
		"RealDrift_NonDefaultOnDefaultedField": {
			desired: baseDesired(),
			current: func() map[string]any {
				c := baseDesired()
				c["ingressFiltering"] = true
				return c
			}(),
			wantNeedsUpdate: true,
		},

		// Create-only field in desired, absent from current — must not
		// produce drift regardless of value.
		"CreateOnlyField_InDesiredNotInCurrent": {
			desired: baseDesired(),
			current: func() map[string]any {
				c := baseDesired()
				delete(c, "region")
				return c
			}(),
			wantNeedsUpdate: false,
		},

		// Legitimate mutable drift on autoScalerMax must surface.
		"LegitimateMutableDrift": {
			desired: baseDesired(),
			current: func() map[string]any {
				c := baseDesired()
				c["autoScalerMax"] = float64(5)
				return c
			}(),
			wantNeedsUpdate: true,
		},

		// accessControlList {} materialised by BTP: no nested defaults but
		// object schema with nested properties. Empty map must count as
		// effective default.
		"AccessControlList_EmptyObjectIsDefault": {
			desired: baseDesired(),
			current: func() map[string]any {
				c := baseDesired()
				c["accessControlList"] = map[string]any{}
				return c
			}(),
			wantNeedsUpdate: false,
		},

		// accessControlList populated with a real CIDR is NOT a default —
		// must surface as drift when spec doesn't set it.
		"AccessControlList_NonEmptyIsDrift": {
			desired: baseDesired(),
			current: func() map[string]any {
				c := baseDesired()
				c["accessControlList"] = map[string]any{
					"allowedCIDRs": []any{"10.0.0.0/8"},
				}
				return c
			}(),
			wantNeedsUpdate: true,
		},

		// gvisor.enabled = true is not default; drift.
		"Gvisor_EnabledTrueIsDrift": {
			desired: baseDesired(),
			current: func() map[string]any {
				c := baseDesired()
				c["gvisor"] = map[string]any{"enabled": true}
				return c
			}(),
			wantNeedsUpdate: true,
		},

		// User explicitly set gvisor.enabled = true. current matches. No
		// drift, and the "user set it" branch of normalizeCurrent must
		// preserve the value.
		"Gvisor_UserSetTrue_CurrentMatches": {
			desired: func() map[string]any {
				d := baseDesired()
				d["gvisor"] = map[string]any{"enabled": true}
				return d
			}(),
			current: func() map[string]any {
				c := baseDesired()
				c["gvisor"] = map[string]any{"enabled": true}
				return c
			}(),
			wantNeedsUpdate: false,
		},

		// Unknown key in current (schema doesn't know about it) must be
		// dropped, not surfaced as drift. Protects against BTP adding an
		// entirely new field without a matching schema update.
		"UnknownKeyInCurrent_Ignored": {
			desired: baseDesired(),
			current: func() map[string]any {
				c := baseDesired()
				c["someBrandNewField"] = "hello"
				return c
			}(),
			wantNeedsUpdate: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			diff, needsUpdate := DiffAgainstUpdateSchema(tc.desired, tc.current, schema)
			if needsUpdate != tc.wantNeedsUpdate {
				t.Errorf("needsUpdate = %v, want %v\ndiff:\n%s",
					needsUpdate, tc.wantNeedsUpdate, diff)
			}
		})
	}
}

// Nil-schema fallback: caller should never pass a nil schema in production,
// but the helper must degrade to the naive comparison rather than panic.
func TestDiffAgainstUpdateSchema_NilSchemaFallsBack(t *testing.T) {
	desired := map[string]any{"a": 1}
	current := map[string]any{"a": 2}

	_, needsUpdate := DiffAgainstUpdateSchema(desired, current, nil)
	if !needsUpdate {
		t.Errorf("nil-schema fallback should detect drift between {a:1} and {a:2}")
	}
}
