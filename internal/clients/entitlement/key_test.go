package entitlement

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
)

// entitlementCR builds a minimal Entitlement CR with the given identity
// fields, leaving every other field at its zero value.
func entitlementCR(subaccountGUID, serviceName, servicePlanName string, qualifier *string) *v1alpha1.Entitlement {
	return &v1alpha1.Entitlement{
		Spec: v1alpha1.EntitlementSpec{
			ForProvider: v1alpha1.EntitlementParameters{
				SubaccountGuid:              subaccountGUID,
				ServiceName:                 serviceName,
				ServicePlanName:             servicePlanName,
				ServicePlanUniqueIdentifier: qualifier,
			},
		},
	}
}

func TestExternalNameRoundTrip(t *testing.T) {
	qualifier := "hana-cloud-hana-sap_eu-de-1"
	cases := map[string]struct {
		key  ExternalNameKey
		want string
	}{
		"three segments": {
			key: ExternalNameKey{
				SubaccountGUID:  "6aa64c2f-38c1-49a9-b2e8-cf9fea769b7f",
				ServiceName:     "alert-notification",
				ServicePlanName: "free",
			},
			want: "6aa64c2f-38c1-49a9-b2e8-cf9fea769b7f/alert-notification/free",
		},
		"four segments": {
			key: ExternalNameKey{
				SubaccountGUID:              "6aa64c2f-38c1-49a9-b2e8-cf9fea769b7f",
				ServiceName:                 "hana-cloud",
				ServicePlanName:             "hana",
				ServicePlanUniqueIdentifier: &qualifier,
			},
			want: "6aa64c2f-38c1-49a9-b2e8-cf9fea769b7f/hana-cloud/hana/hana-cloud-hana-sap_eu-de-1",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.key.String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
			got, err := ParseExternalName(tc.want)
			if err != nil {
				t.Fatalf("ParseExternalName() error = %v", err)
			}
			if diff := cmp.Diff(tc.key, got); diff != "" {
				t.Fatalf("ParseExternalName() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseExternalNameRejectsInvalidInput(t *testing.T) {
	cases := map[string]struct {
		value   string
		wantErr error
	}{
		"too few segments": {
			value:   "a/b",
			wantErr: ErrInvalidExternalName,
		},
		"too many segments": {
			value:   "a/b/c/d/e",
			wantErr: ErrInvalidExternalName,
		},
		"empty first segment": {
			value:   "/b/c",
			wantErr: ErrEmptyExternalNameSegment,
		},
		"empty middle segment": {
			value:   "a//c",
			wantErr: ErrEmptyExternalNameSegment,
		},
		"leading whitespace in segment": {
			value:   "a/b/ c",
			wantErr: ErrEmptyExternalNameSegment,
		},
		"blank fourth segment": {
			value:   "a/b/c/",
			wantErr: ErrEmptyExternalNameSegment,
		},
		"exceeds max length": {
			value:   strings.Repeat("a", 513),
			wantErr: ErrExternalNameTooLong,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseExternalName(tc.value)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ParseExternalName(%q) error = %v, want errors.Is(_, %v)", tc.value, err, tc.wantErr)
			}
		})
	}
}

func TestParseExternalNameMaxLength(t *testing.T) {
	// 170 + 1 + 170 + 1 + 170 == 512, the maximum accepted length.
	value := strings.Repeat("a", 170) + "/" + strings.Repeat("b", 170) + "/" + strings.Repeat("c", 170)
	if len(value) != externalNameMaxLen {
		t.Fatalf("test fixture length = %d, want %d", len(value), externalNameMaxLen)
	}
	got, err := ParseExternalName(value)
	if err != nil {
		t.Fatalf("ParseExternalName() error = %v, want nil", err)
	}
	want := ExternalNameKey{
		SubaccountGUID:  strings.Repeat("a", 170),
		ServiceName:     strings.Repeat("b", 170),
		ServicePlanName: strings.Repeat("c", 170),
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("ParseExternalName() mismatch (-want +got):\n%s", diff)
	}
}

func TestExternalNameMismatch(t *testing.T) {
	qual1, qual2 := "qual-1", "qual-2"
	matchingKey := ExternalNameKey{
		SubaccountGUID:              "sa-1",
		ServiceName:                 "svc-1",
		ServicePlanName:             "plan-1",
		ServicePlanUniqueIdentifier: internal.Ptr(qual1),
	}
	cr := entitlementCR("sa-1", "svc-1", "plan-1", internal.Ptr(qual1))

	cases := map[string]struct {
		key  ExternalNameKey
		cr   *v1alpha1.Entitlement
		want string
	}{
		"matching key and spec produce no mismatch": {
			key:  matchingKey,
			cr:   cr,
			want: "",
		},
		"subaccountGuid differs": {
			key: ExternalNameKey{
				SubaccountGUID:              "sa-2",
				ServiceName:                 "svc-1",
				ServicePlanName:             "plan-1",
				ServicePlanUniqueIdentifier: &qual1,
			},
			cr:   cr,
			want: `subaccountGuid mismatch (annotation="sa-2", spec="sa-1")`,
		},
		"serviceName differs": {
			key: ExternalNameKey{
				SubaccountGUID:              "sa-1",
				ServiceName:                 "svc-2",
				ServicePlanName:             "plan-1",
				ServicePlanUniqueIdentifier: &qual1,
			},
			cr:   cr,
			want: `serviceName mismatch (annotation="svc-2", spec="svc-1")`,
		},
		"servicePlanName differs": {
			key: ExternalNameKey{
				SubaccountGUID:              "sa-1",
				ServiceName:                 "svc-1",
				ServicePlanName:             "plan-2",
				ServicePlanUniqueIdentifier: &qual1,
			},
			cr:   cr,
			want: `servicePlanName mismatch (annotation="plan-2", spec="plan-1")`,
		},
		"qualifier value differs": {
			key: ExternalNameKey{
				SubaccountGUID:              "sa-1",
				ServiceName:                 "svc-1",
				ServicePlanName:             "plan-1",
				ServicePlanUniqueIdentifier: &qual2,
			},
			cr:   cr,
			want: `servicePlanUniqueIdentifier mismatch (annotation="qual-2", spec="qual-1")`,
		},
		"qualifier unset on annotation side": {
			key: ExternalNameKey{
				SubaccountGUID:  "sa-1",
				ServiceName:     "svc-1",
				ServicePlanName: "plan-1",
			},
			cr:   cr,
			want: `servicePlanUniqueIdentifier mismatch (annotation=<unset>, spec="qual-1")`,
		},
		"qualifier unset on spec side": {
			key:  matchingKey,
			cr:   entitlementCR("sa-1", "svc-1", "plan-1", nil),
			want: `servicePlanUniqueIdentifier mismatch (annotation="qual-1", spec=<unset>)`,
		},
		"multiple mismatches are semicolon joined": {
			key: ExternalNameKey{
				SubaccountGUID:              "sa-2",
				ServiceName:                 "svc-2",
				ServicePlanName:             "plan-1",
				ServicePlanUniqueIdentifier: &qual1,
			},
			cr: cr,
			want: `subaccountGuid mismatch (annotation="sa-2", spec="sa-1"); ` +
				`serviceName mismatch (annotation="svc-2", spec="svc-1")`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.key.Mismatch(tc.cr); got != tc.want {
				t.Fatalf("Mismatch() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCacheKeyExcludesQualifier(t *testing.T) {
	qual1, qual2 := "qual-1", "qual-2"
	withQual1 := ExternalNameKey{SubaccountGUID: "sa", ServiceName: "svc", ServicePlanName: "plan", ServicePlanUniqueIdentifier: &qual1}
	withQual2 := ExternalNameKey{SubaccountGUID: "sa", ServiceName: "svc", ServicePlanName: "plan", ServicePlanUniqueIdentifier: &qual2}
	withoutQual := ExternalNameKey{SubaccountGUID: "sa", ServiceName: "svc", ServicePlanName: "plan"}

	if withQual1.CacheKey() != withQual2.CacheKey() {
		t.Fatalf("CacheKey() differs for keys that only differ by qualifier: %q vs %q", withQual1.CacheKey(), withQual2.CacheKey())
	}
	if withQual1.CacheKey() != withoutQual.CacheKey() {
		t.Fatalf("CacheKey() differs between a present and an absent qualifier: %q vs %q", withQual1.CacheKey(), withoutQual.CacheKey())
	}
}

func TestNewExternalNameKeyRejectsInvalidSpec(t *testing.T) {
	tooLong := strings.Repeat("a", 510)
	cases := map[string]struct {
		cr      *v1alpha1.Entitlement
		wantErr error
	}{
		"empty required segment": {
			cr:      entitlementCR("", "svc", "plan", nil),
			wantErr: ErrEmptyExternalNameSegment,
		},
		"slash in otherwise valid three-segment field": {
			cr:      entitlementCR("sa", "alert/notification", "plan", nil),
			wantErr: ErrInvalidExternalName,
		},
		"leading whitespace": {
			cr:      entitlementCR(" sa", "svc", "plan", nil),
			wantErr: ErrEmptyExternalNameSegment,
		},
		"present empty qualifier": {
			cr:      entitlementCR("sa", "svc", "plan", internal.Ptr("")),
			wantErr: ErrEmptyExternalNameSegment,
		},
		"slash in qualifier": {
			cr:      entitlementCR("sa", "svc", "plan", internal.Ptr("qual/extra")),
			wantErr: ErrInvalidExternalName,
		},
		"compound value over max length": {
			cr:      entitlementCR(tooLong, "b", "c", nil),
			wantErr: ErrExternalNameTooLong,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewExternalNameKey(tc.cr)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("NewExternalNameKey() error = %v, want errors.Is(_, %v)", err, tc.wantErr)
			}
		})
	}
}

func TestBuildExternalNameRoundTrip(t *testing.T) {
	qualifier := "plan-qualifier"
	cases := map[string]struct {
		cr   *v1alpha1.Entitlement
		want string
	}{
		"three segments": {
			cr:   entitlementCR("sa-guid", "svc-name", "plan-name", nil),
			want: "sa-guid/svc-name/plan-name",
		},
		"four segments": {
			cr:   entitlementCR("sa-guid", "svc-name", "plan-name", &qualifier),
			want: "sa-guid/svc-name/plan-name/plan-qualifier",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := BuildExternalName(tc.cr)
			if err != nil {
				t.Fatalf("BuildExternalName() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("BuildExternalName() = %q, want %q", got, tc.want)
			}
			parsed, err := ParseExternalName(got)
			if err != nil {
				t.Fatalf("ParseExternalName(%q) error = %v", got, err)
			}
			wantKey, err := NewExternalNameKey(tc.cr)
			if err != nil {
				t.Fatalf("NewExternalNameKey() error = %v", err)
			}
			if diff := cmp.Diff(wantKey, parsed); diff != "" {
				t.Fatalf("round-trip mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
