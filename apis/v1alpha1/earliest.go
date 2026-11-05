package v1alpha1

import "time"

// Earliest returns the earliest of ts, or false if ts is empty.
func Earliest(ts ...time.Time) (time.Time, bool) {
	if len(ts) == 0 {
		return time.Time{}, false
	}
	at := ts[0]
	for _, t := range ts[1:] {
		if t.Before(at) {
			at = t
		}
	}
	return at, true
}
