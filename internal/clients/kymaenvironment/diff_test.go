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

		// Reporter's workaround: the same keys set explicitly in desired.
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

		// A non-default value on a schema-defaulted field IS real drift.
		"RealDrift_NonDefaultOnDefaultedField": {
			desired: baseDesired(),
			current: func() map[string]any {
				c := baseDesired()
				c["ingressFiltering"] = true
				return c
			}(),
			wantNeedsUpdate: true,
		},

		// Create-only field in desired, absent from current: never drift.
		"CreateOnlyField_InDesiredNotInCurrent": {
			desired: baseDesired(),
			current: func() map[string]any {
				c := baseDesired()
				delete(c, "region")
				return c
			}(),
			wantNeedsUpdate: false,
		},

		"LegitimateMutableDrift": {
			desired: baseDesired(),
			current: func() map[string]any {
				c := baseDesired()
				c["autoScalerMax"] = float64(5)
				return c
			}(),
			wantNeedsUpdate: true,
		},

		// accessControlList {} materialised by BTP: object schema with nested
		// properties but no explicit default. Empty map counts as default.
		"AccessControlList_EmptyObjectIsDefault": {
			desired: baseDesired(),
			current: func() map[string]any {
				c := baseDesired()
				c["accessControlList"] = map[string]any{}
				return c
			}(),
			wantNeedsUpdate: false,
		},

		// A populated accessControlList is not the default: drift.
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

		"Gvisor_EnabledTrueIsDrift": {
			desired: baseDesired(),
			current: func() map[string]any {
				c := baseDesired()
				c["gvisor"] = map[string]any{"enabled": true}
				return c
			}(),
			wantNeedsUpdate: true,
		},

		// User set gvisor.enabled=true and current matches: the "user set it"
		// branch must preserve the value so no false drift appears.
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

		// Unknown key in current (not in schema) must be ignored, not drift.
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

func TestFilterToUpdateSchema(t *testing.T) {
	schema := kymaAzureSchemaForDiffTest()

	t.Run("DropsCreateOnlyFields", func(t *testing.T) {
		in := baseDesired() // includes create-only "region"
		out := FilterToUpdateSchema(in, schema)
		if _, ok := out["region"]; ok {
			t.Errorf("region should have been dropped; got %v", out)
		}
		// A shared/updatable field must survive.
		if _, ok := out["machineType"]; !ok {
			t.Errorf("machineType should have survived; got %v", out)
		}
	})

	t.Run("KeepsInSchemaFields", func(t *testing.T) {
		in := map[string]any{
			"name":             "kyma",
			"ingressFiltering": false,
			"gvisor":           map[string]any{"enabled": false},
		}
		out := FilterToUpdateSchema(in, schema)
		for _, k := range []string{"name", "ingressFiltering", "gvisor"} {
			if _, ok := out[k]; !ok {
				t.Errorf("%q should have survived; got %v", k, out)
			}
		}
	})

	t.Run("NilSchemaPassesThrough", func(t *testing.T) {
		in := map[string]any{"region": "westeurope"}
		out := FilterToUpdateSchema(in, nil)
		if _, ok := out["region"]; !ok {
			t.Errorf("nil schema must pass params through unchanged; got %v", out)
		}
	})
}
