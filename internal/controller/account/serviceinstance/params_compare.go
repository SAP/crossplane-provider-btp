package serviceinstance

import (
	"encoding/json"
)

// paramsSubsetMatch reports whether every key in desired is present in server
// with an equal (deep, order-independent) value. Keys that exist only on the
// server side are ignored.
//
// This asymmetry is deliberate and is what makes reading BTP service-instance
// parameters safe as a drift signal despite the two constraints the BTP
// Terraform provider maintainers called out (terraform-provider-btp#1643):
//
//  1. Not every offering returns parameters — handled by the caller, which
//     skips the comparison entirely when the server returns no parameters.
//  2. There is no contract that returned parameters match the create/update
//     schema — the server may add defaults or extra fields. A full equality
//     check would flag those as drift forever. Subset matching only asserts
//     "everything I asked for is in effect", never "the server looks exactly
//     like my request", so server-added defaults never cause phantom drift.
//
// Comparison is structural (both sides are decoded from JSON into any), so
// object key ordering is irrelevant and nested objects are compared recursively.
//
// SCOPE — ADD/CHANGE ONLY, NO DELETE. This detects a desired parameter that is
// added or changed relative to the server, but deliberately does NOT detect a
// parameter that was previously set and is now removed from the desired spec.
// Reason (verified empirically against a live SAP BTP object-store broker,
// July 2026): parameter deletion is not reliably expressible over the Service
// Manager update API.
//   - Sending an empty object {} is a no-op — the broker keeps all existing
//     parameters.
//   - Sending a non-empty object that omits a previously-set key does NOT reset
//     that key; the broker merges, keeping the old value.
//   - Sending an explicit null for a key crashes the broker (HTTP 500).
//
// Because a removed key cannot be cleared through an update, forcing drift on
// deletion would either loop forever (update sent, server unchanged, drift
// re-detected) or fail the update outright. The only reliable way to remove a
// parameter is to recreate the instance. Deletion is therefore left to the
// normal spec-driven recreate path, not to this drift signal.
func paramsSubsetMatch(desired, server map[string]any) bool {
	for k, dv := range desired {
		sv, ok := server[k]
		if !ok {
			return false
		}
		if !deepValueEqual(dv, sv) {
			return false
		}
	}
	return true
}

// deepValueEqual compares two JSON-decoded values. Objects are compared as a
// subset (desired ⊆ server) recursively; arrays and scalars require full
// equality. Numbers are compared via their JSON encoding so that, e.g., an
// int-typed desired value and a float64-typed decoded server value that
// represent the same number compare equal.
func deepValueEqual(desired, server any) bool {
	switch dv := desired.(type) {
	case map[string]any:
		sv, ok := server.(map[string]any)
		if !ok {
			return false
		}
		return paramsSubsetMatch(dv, sv)
	case []any:
		sv, ok := server.([]any)
		if !ok || len(dv) != len(sv) {
			return false
		}
		for i := range dv {
			if !deepValueEqual(dv[i], sv[i]) {
				return false
			}
		}
		return true
	default:
		return scalarEqual(desired, server)
	}
}

// scalarEqual compares two scalar JSON values tolerant of Go type differences
// introduced by json.Unmarshal (e.g. json.Number vs float64 vs int).
func scalarEqual(a, b any) bool {
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(ab) == string(bb)
}
