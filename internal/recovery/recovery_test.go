package recovery

import (
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// crWithPending builds a minimal metav1.Object with a CR creationTimestamp
// and optionally an `crossplane.io/external-create-pending` annotation set
// via crossplane-runtime's meta helper. Passing a zero pendingAt means "no
// pending annotation on the CR" (never attempted Create).
func crWithPending(crCreatedAt, pendingAt time.Time) metav1.Object {
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.NewTime(crCreatedAt),
		},
	}
	if !pendingAt.IsZero() {
		meta.SetExternalCreatePending(obj, pendingAt)
	}
	return obj
}

// TestIsOwnedByCR_RequiresPendingAnnotation locks in the safety property that
// motivated dropping the earlier creationTimestamp fallback: without a
// pending annotation we have no evidence THIS controller ever attempted a
// Create for this CR, so we cannot safely adopt a same-named BTP resource
// — a UI-created same-named resource that appeared any time later would
// have been pulled in. crossplane-runtime writes external-create-pending
// before every Create (and aborts if that write fails), so a missing
// annotation is a reliable "never attempted" signal.
func TestIsOwnedByCR_RequiresPendingAnnotation(t *testing.T) {
	crCreated := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	cr := crWithPending(crCreated, time.Time{})

	tests := []struct {
		name         string
		btpCreatedAt time.Time
	}{
		{name: "btp at CR creationTimestamp: refused (no create-pending)", btpCreatedAt: crCreated},
		{name: "btp minutes after CR: refused (no create-pending)", btpCreatedAt: crCreated.Add(3 * time.Minute)},
		{name: "btp days after CR (same-name resource created later): refused (no create-pending)", btpCreatedAt: crCreated.Add(48 * time.Hour)},
		{name: "btp 30s before CR: refused (no create-pending)", btpCreatedAt: crCreated.Add(-30 * time.Second)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if IsOwnedByCR(cr, tc.btpCreatedAt) {
				t.Errorf("IsOwnedByCR without create-pending must always be false; got true for btp=%v", tc.btpCreatedAt)
			}
		})
	}
}

// TestIsOwnedByCR_PendingWindow covers the bounded window around the
// recorded Create-attempt time — [pending-clockSkew, pending+upperWindow].
// A matching resource should be born close to the attempt; anything much
// later (beyond the async-provisioning upper window) cannot be the result of
// our attempt and is refused.
func TestIsOwnedByCR_PendingWindow(t *testing.T) {
	crCreated := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	// Simulate a real recovery: CR created at 10:00, Create actually attempted
	// at 16:00 (e.g. after a long ref-resolution delay).
	attemptAt := time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC)
	cr := crWithPending(crCreated, attemptAt)

	tests := []struct {
		name         string
		btpCreatedAt time.Time
		want         bool
	}{
		{name: "btp exactly at attempt: ours", btpCreatedAt: attemptAt, want: true},
		{name: "btp attempt+3s (normal BTP roundtrip): ours", btpCreatedAt: attemptAt.Add(3 * time.Second), want: true},
		{name: "btp attempt+30m (slow async provisioning, still within upper window): ours", btpCreatedAt: attemptAt.Add(30 * time.Minute), want: true},
		{name: "btp attempt+1h (exactly upper boundary): ours", btpCreatedAt: attemptAt.Add(1 * time.Hour), want: true},
		{name: "btp attempt+1h1m (past upper window): refused as someone-else-later", btpCreatedAt: attemptAt.Add(61 * time.Minute), want: false},
		{name: "btp attempt-30s (within clock skew): ours", btpCreatedAt: attemptAt.Add(-30 * time.Second), want: true},
		{name: "btp attempt-60s (skew boundary): ours", btpCreatedAt: attemptAt.Add(-60 * time.Second), want: true},
		{name: "btp attempt-61s (past lower skew): refused as brownfield", btpCreatedAt: attemptAt.Add(-61 * time.Second), want: false},
		{name: "btp at CR creation (6h before attempt): refused as brownfield", btpCreatedAt: crCreated, want: false},
		{name: "btp 2h after CR / 4h before attempt: refused as brownfield", btpCreatedAt: crCreated.Add(2 * time.Hour), want: false},
		{name: "unknown btp created_at (zero): refused", btpCreatedAt: time.Time{}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsOwnedByCR(cr, tc.btpCreatedAt); got != tc.want {
				t.Errorf("IsOwnedByCR(pending=%v, btp=%v) = %v, want %v",
					attemptAt, tc.btpCreatedAt, got, tc.want)
			}
		})
	}
}

// TestIsTruncatedCompoundExternalName: a compound external-name
// (instanceID/bindingID) truncated to the bare instanceID, distinguished from a
// fallback and from a healthy compound name.
func TestIsTruncatedCompoundExternalName(t *testing.T) {
	tests := []struct {
		name         string
		metadataName string
		externalName string
		want         bool
	}{
		{name: "bare instance UUID (truncated compound) is truncated", metadataName: "cis-abc", externalName: "11111111-1111-1111-1111-111111111111", want: true},
		{name: "healthy compound instanceID/bindingID is not truncated", metadataName: "cis-abc", externalName: "11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222", want: false},
		{name: "empty external-name is not truncated (it is a fallback)", metadataName: "cis-abc", externalName: "", want: false},
		{name: "external-name equal to metadata.name is not truncated (it is a fallback)", metadataName: "cis-abc", externalName: "cis-abc", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTruncatedCompoundExternalName(tc.metadataName, tc.externalName); got != tc.want {
				t.Errorf("IsTruncatedCompoundExternalName(%q,%q) = %v, want %v", tc.metadataName, tc.externalName, got, tc.want)
			}
		})
	}
}

// TestIsOwnedByExternalNameInstanceID: the instance UUID our phase-1 Create
// wrote into external-name must equal the instance the lookup found. Used
// instead of the time window on the truncated-compound path.
func TestIsOwnedByExternalNameInstanceID(t *testing.T) {
	tests := []struct {
		name                 string
		externalNameInstance string
		foundInstance        string
		want                 bool
	}{
		{name: "matching non-empty instance IDs: owned", externalNameInstance: "11111111-1111-1111-1111-111111111111", foundInstance: "11111111-1111-1111-1111-111111111111", want: true},
		{name: "different instance IDs: not owned", externalNameInstance: "11111111-1111-1111-1111-111111111111", foundInstance: "33333333-3333-3333-3333-333333333333", want: false},
		{name: "empty external-name instance ID: not owned", externalNameInstance: "", foundInstance: "11111111-1111-1111-1111-111111111111", want: false},
		{name: "both empty: not owned", externalNameInstance: "", foundInstance: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsOwnedByExternalNameInstanceID(tc.externalNameInstance, tc.foundInstance); got != tc.want {
				t.Errorf("IsOwnedByExternalNameInstanceID(%q,%q) = %v, want %v", tc.externalNameInstance, tc.foundInstance, got, tc.want)
			}
		})
	}
}

// TestHasCreateBeenAttempted is a thin wrapper around
// meta.GetExternalCreatePending; it exists so callers can name-check the
// intent (short-circuit before running a semantic lookup) rather than
// leaking annotation plumbing into every controller.
func TestHasCreateBeenAttempted(t *testing.T) {
	crCreated := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	pendingAt := time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC)

	t.Run("no pending annotation: false", func(t *testing.T) {
		if HasCreateBeenAttempted(crWithPending(crCreated, time.Time{})) {
			t.Errorf("HasCreateBeenAttempted = true, want false")
		}
	})

	t.Run("pending annotation set: true", func(t *testing.T) {
		if !HasCreateBeenAttempted(crWithPending(crCreated, pendingAt)) {
			t.Errorf("HasCreateBeenAttempted = false, want true")
		}
	})
}
