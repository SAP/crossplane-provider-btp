package environments

import (
	"reflect"

	"github.com/google/go-cmp/cmp"
)

// DiffAgainstUpdateSchema compares desired to current, restricted to the
// property surface defined by the BTP updateSchema. Keys outside
// schema.Properties are ignored on both sides (they represent create-only or
// otherwise non-updatable fields). For keys the user did not set in desired,
// observed values in current are treated as absent when they equal the
// schema's effective default — this suppresses the false-drift condition
// where BTP materialises schema defaults into stored parameters after any
// update.
//
// Returns a cmp.Diff-formatted string suitable for surfacing on
// KymaEnvironment.status.updateRetryStatus.diff, and a bool indicating
// whether an update is required.
//
// See issue https://github.com/SAP/crossplane-provider-btp/issues/682 for the
// full motivation.
func DiffAgainstUpdateSchema(desired, current map[string]any, schema *Schema) (string, bool) {
	if schema == nil {
		// Without a schema we can't reason about defaults or the update
		// contract. Fall back to the strict comparison the caller had before
		// this helper existed. (In practice needsUpdateWithDiff should fail
		// closed before ever reaching this helper with a nil schema.)
		diff := cmp.Diff(desired, current)
		return diff, diff != ""
	}

	restrictedDesired := restrictToSchema(desired, schema)
	normalizedCurrent := normalizeCurrent(desired, current, schema)

	diff := cmp.Diff(restrictedDesired, normalizedCurrent)
	return diff, diff != ""
}

// restrictToSchema copies m, keeping only keys named in schema.Properties.
// Keys outside the schema (create-only fields, unknown extras) are dropped.
// The result is always non-nil, even if empty, to keep cmp.Diff output
// stable.
func restrictToSchema(m map[string]any, schema *Schema) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		if _, ok := schema.Properties[k]; ok {
			out[k] = v
		}
	}
	return out
}

// normalizeCurrent builds a version of current suitable for direct comparison
// against restrictedDesired. Keys outside schema.Properties are dropped
// (mirrors restrictToSchema). For keys not set in desired, values matching
// the schema's effective default are dropped so they don't manifest as drift.
func normalizeCurrent(desired, current map[string]any, schema *Schema) map[string]any {
	out := map[string]any{}
	for k, v := range current {
		prop, ok := schema.Properties[k]
		if !ok {
			// Not in update contract — ignore regardless of value.
			continue
		}
		if _, userSet := desired[k]; userSet {
			// User expressed intent about this key; compare directly.
			out[k] = v
			continue
		}
		if matchesEffectiveDefault(v, prop) {
			// User didn't set it and BTP echoed the schema default; treat
			// as absent so it doesn't produce false drift.
			continue
		}
		// User didn't set it, but BTP has a non-default value here.
		// Preserve — this is real drift that deserves surfacing.
		out[k] = v
	}
	return out
}

// matchesEffectiveDefault reports whether v is indistinguishable from the
// schema-declared default for prop. The rules, in order:
//
//   - If prop has an explicit Default, compare v to it with reflect.DeepEqual.
//   - If prop is an object type with nested Properties (no explicit Default):
//     empty maps and maps where every present key recursively matches its
//     nested property's effective default count as the effective default.
//   - Otherwise: v never matches "the default" because no default exists.
func matchesEffectiveDefault(v any, prop Property) bool {
	if prop.Default != nil {
		return reflect.DeepEqual(v, prop.Default)
	}

	if prop.Type == "object" && prop.Properties != nil {
		m, ok := v.(map[string]any)
		if !ok {
			return false
		}
		for k, sub := range m {
			nested, ok := prop.Properties[k]
			if !ok {
				// Unexpected key in observed object — not a match.
				return false
			}
			if !matchesEffectiveDefault(sub, nested) {
				return false
			}
		}
		return true
	}

	return false
}
