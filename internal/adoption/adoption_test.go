package adoption

import (
	"testing"
	"time"
)

func TestIsFallbackExternalName(t *testing.T) {
	tests := []struct {
		name         string
		metadataName string
		externalName string
		want         bool
	}{
		{name: "empty external-name is fallback", metadataName: "cls-abc", externalName: "", want: true},
		{name: "external-name equal to metadata.name is fallback", metadataName: "cls-abc", externalName: "cls-abc", want: true},
		{name: "real UUID external-name is not fallback", metadataName: "cls-abc", externalName: "80540c06-2955-4bce-9c43-ad78fecc7f62", want: false},
		{name: "different non-UUID external-name is not fallback", metadataName: "cls-abc", externalName: "user-typed-value", want: false},
		// Regression: single-UUID external-name (SM/CM two-phase-create phase-1
		// output) must NOT be treated as fallback. Earlier drafts had an
		// IsFallbackOrNonCompound helper that flipped this to true, which
		// re-fired adoption every reconcile and prevented phase-2 (createBinding)
		// from running — SM/CM were trapped with a single-UUID external-name
		// and no binding was ever created. The compound-key test now lives in
		// the SM/CM controllers, not in the adoption trigger.
		{name: "single UUID (SM/CM phase-1 output) is not fallback", metadataName: "sm-abc", externalName: "80540c06-2955-4bce-9c43-ad78fecc7f62", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsFallbackExternalName(tc.metadataName, tc.externalName); got != tc.want {
				t.Errorf("IsFallbackExternalName(%q,%q) = %v, want %v", tc.metadataName, tc.externalName, got, tc.want)
			}
		})
	}
}

func TestIsOwnedByCR(t *testing.T) {
	// Fixed reference point so the tests are deterministic.
	crCreated := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		btpCreatedAt time.Time
		want         bool
	}{
		{
			name:         "btp created well after CR: ours",
			btpCreatedAt: crCreated.Add(30 * time.Second),
			want:         true,
		},
		{
			name:         "btp created just after CR: ours",
			btpCreatedAt: crCreated.Add(1 * time.Second),
			want:         true,
		},
		{
			name:         "btp created exactly at CR: ours (equality)",
			btpCreatedAt: crCreated,
			want:         true,
		},
		{
			name:         "btp created 30s before CR: within skew tolerance, treat as ours",
			btpCreatedAt: crCreated.Add(-30 * time.Second),
			want:         true,
		},
		{
			name:         "btp created 60s before CR: right at skew boundary, treat as ours",
			btpCreatedAt: crCreated.Add(-60 * time.Second),
			want:         true,
		},
		{
			name:         "btp created 61s before CR: outside skew tolerance, brownfield",
			btpCreatedAt: crCreated.Add(-61 * time.Second),
			want:         false,
		},
		{
			name:         "btp created 1h before CR: obvious brownfield",
			btpCreatedAt: crCreated.Add(-1 * time.Hour),
			want:         false,
		},
		{
			name:         "btp created 30d before CR: obvious brownfield",
			btpCreatedAt: crCreated.Add(-30 * 24 * time.Hour),
			want:         false,
		},
		{
			// Server didn't populate created_at (missing/omitted). Fail closed:
			// unknown provenance is refused, forcing the user to import
			// explicitly if they really want to adopt.
			name:         "unknown btp created_at (zero-value): refused",
			btpCreatedAt: time.Time{},
			want:         false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsOwnedByCR(crCreated, tc.btpCreatedAt); got != tc.want {
				t.Errorf("IsOwnedByCR(cr=%v, btp=%v) = %v, want %v",
					crCreated, tc.btpCreatedAt, got, tc.want)
			}
		})
	}
}
