package serviceinstanceclient

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ParameterDrift performs an asymmetric, recursive three-way comparison of the
// parameters a user desires (desired), the parameters we last successfully
// applied to the Service Manager (lastApplied), and the parameters BTP reports
// back (observed). It returns whether the desired state has drifted from what
// BTP holds and a human-readable path describing the first drift found.
//
// The comparison is asymmetric because BTP augments and mutates the parameters
// we send: keys present in the observed set but neither desired nor last
// applied are BTP extras and are ignored. The lastApplied leg enables detecting
// keys the user removed from spec (desired absent, but we applied it before and
// BTP still holds it => drift, converges because SM PATCH replaces the whole
// parameter object).
//
// Per-key verdicts:
//
//	desired present, observed present   -> drift iff values differ (exact value compare)
//	desired present, observed absent    -> no drift (redaction-safe)
//	desired absent, lastApplied present, observed present -> drift (user removed key)
//	desired absent, lastApplied absent, observed present  -> no drift (BTP extra)
//	desired absent, lastApplied present, observed absent  -> no drift (already converged)
//	object vs object                    -> recurse (three-way)
//	array vs array                      -> order-invariant multiset deep-equal
//	shape mismatch (obj vs scalar, ...) -> drift
func ParameterDrift(desired, lastApplied, observed map[string]interface{}) (bool, string) {
	return driftAtMap("", desired, lastApplied, observed)
}

// driftAtMap walks the union of desired and observed keys at one level and
// applies the three-way per-key verdict. It returns on the first drift found so
// the reported path is deterministic (keys are visited in sorted order).
func driftAtMap(path string, desired, lastApplied, observed map[string]interface{}) (bool, string) {
	for _, k := range unionKeys(desired, observed) {
		d, dOK := desired[k]
		l, lOK := lastApplied[k]
		o, oOK := observed[k]
		child := joinPath(path, k)

		if dOK {
			// Desired has this key.
			if !oOK {
				// Observed absent: redaction-safe -> no drift (a redacted /
				// write-only param that BTP omits must not read as drift).
				continue
			}
			if drift, diff := valueDrift(child, d, l, o); drift {
				return true, diff
			}
			continue
		}

		// Desired does NOT have this key.
		if lOK && oOK {
			// We applied it before and BTP still holds it: the user removed it
			// from spec -> drift. Converges because SM PATCH replaces params.
			return true, fmt.Sprintf("%s: removed from spec but still present in Service Manager", child)
		}
		// Otherwise it's a BTP extra (absent/absent/present) or already
		// converged (present/absent) -> no drift.
	}
	return false, ""
}

// valueDrift compares a single desired value against the observed value at the
// given path. Both-map values recurse (carrying the last-applied sub-map for
// nested removal detection); everything else is compared by order-invariant
// canonical equality, which also catches shape mismatches.
func valueDrift(path string, desired, lastApplied, observed interface{}) (bool, string) {
	dMap, dIsMap := desired.(map[string]interface{})
	oMap, oIsMap := observed.(map[string]interface{})
	if dIsMap && oIsMap {
		lMap, _ := lastApplied.(map[string]interface{})
		return driftAtMap(path, dMap, lMap, oMap)
	}

	if canonicalValue(desired) == canonicalValue(observed) {
		return false, ""
	}
	return true, fmt.Sprintf("%s: desired %s != observed %s", path, canonicalValue(desired), canonicalValue(observed))
}

// canonicalValue produces a deterministic, order-invariant string for any
// JSON-decoded value. Maps are emitted with sorted keys; arrays are compared as
// multisets (each element canonicalized, then the element strings sorted), so
// element order is ignored recursively. This is used for leaf/array equality
// and never for the map-level three-way walk (which must stay asymmetric).
func canonicalValue(v interface{}) string {
	switch t := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			b.Write(kb)
			b.WriteByte(':')
			b.WriteString(canonicalValue(t[k]))
		}
		b.WriteByte('}')
		return b.String()
	case []interface{}:
		elems := make([]string, 0, len(t))
		for _, e := range t {
			elems = append(elems, canonicalValue(e))
		}
		sort.Strings(elems)
		return "[" + strings.Join(elems, ",") + "]"
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// unionKeys returns the sorted union of the keys of two maps.
func unionKeys(a, b map[string]interface{}) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// CanonicalParameterJSON serializes a parameter map to canonical JSON (sorted
// keys via encoding/json) for storage as the last-applied / pending snapshot.
// Returns "" for a nil/empty map so an empty snapshot round-trips cleanly.
func CanonicalParameterJSON(m map[string]interface{}) (string, error) {
	if len(m) == 0 {
		return "", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeParameterJSON parses a canonical parameter JSON snapshot back into a
// map. An empty string decodes to a nil map (no snapshot).
func DecodeParameterJSON(s string) (map[string]interface{}, error) {
	if s == "" {
		return nil, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return m, nil
}
