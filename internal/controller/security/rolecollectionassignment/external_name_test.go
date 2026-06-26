package rolecollectionassignment

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBuildExternalName(t *testing.T) {
	cases := map[string]struct {
		modifiers []RoleCollectionModifier
		want      string
	}{
		"User": {
			modifiers: []RoleCollectionModifier{withUser("alice@example.com"), withOrigin("sap.default"), withRoleCollection("Subaccount Viewer")},
			want:      "sap.default/alice@example.com/Subaccount Viewer",
		},
		"Group": {
			modifiers: []RoleCollectionModifier{withGroup("BTP Admins"), withOrigin("sap.default"), withRoleCollection("Subaccount Administrator")},
			want:      "sap.default/BTP Admins/Subaccount Administrator",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := BuildExternalName(cr(tc.modifiers...))
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("BuildExternalName(): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestParseExternalName_Valid(t *testing.T) {
	cases := map[string]struct {
		in         string
		wantOrigin string
		wantName   string
		wantRC     string
	}{
		"user":  {in: "sap.default/alice@example.com/Subaccount Viewer", wantOrigin: "sap.default", wantName: "alice@example.com", wantRC: "Subaccount Viewer"},
		"group": {in: "sap.default/BTP Admins/Subaccount Administrator", wantOrigin: "sap.default", wantName: "BTP Admins", wantRC: "Subaccount Administrator"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			origin, gotName, rc, err := ParseExternalName(tc.in)
			if err != nil {
				t.Fatalf("ParseExternalName(%q) unexpected err: %v", tc.in, err)
			}
			if origin != tc.wantOrigin || gotName != tc.wantName || rc != tc.wantRC {
				t.Errorf("ParseExternalName(%q) = (%q,%q,%q), want (%q,%q,%q)",
					tc.in, origin, gotName, rc, tc.wantOrigin, tc.wantName, tc.wantRC)
			}
		})
	}
}

func TestParseExternalName_Invalid(t *testing.T) {
	cases := map[string]struct {
		in              string
		wantErrContains string
	}{
		"empty":              {in: "", wantErrContains: "format"},
		"two parts":          {in: "sap.default/alice@example.com", wantErrContains: "format"},
		"four parts":         {in: "sap.default/alice@example.com/Subaccount/Viewer", wantErrContains: "format"},
		"empty origin":       {in: "/alice@example.com/Subaccount Viewer", wantErrContains: "empty"},
		"empty middle":       {in: "sap.default//Subaccount Viewer", wantErrContains: "empty"},
		"empty rc":           {in: "sap.default/alice@example.com/", wantErrContains: "empty"},
		"leading whitespace": {in: " sap.default/alice@example.com/Subaccount Viewer", wantErrContains: "empty"},
		"trailing newline":   {in: "sap.default/alice@example.com/Subaccount Viewer\n", wantErrContains: "empty"},
		"too long":           {in: strings.Repeat("a", externalNameMaxLen+1), wantErrContains: "maximum"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, _, err := ParseExternalName(tc.in)
			if err == nil {
				t.Fatalf("ParseExternalName(%q) = nil error, want error containing %q", tc.in, tc.wantErrContains)
			}
			if !strings.Contains(err.Error(), tc.wantErrContains) {
				t.Errorf("ParseExternalName(%q) err = %q, want substring %q", tc.in, err.Error(), tc.wantErrContains)
			}
		})
	}
}

func TestBuildParseRoundtrip(t *testing.T) {
	c := cr(withUser("bob@example.com"), withOrigin("sap.default"), withRoleCollection("Subaccount Service Administrator"))
	out := BuildExternalName(c)
	origin, name, rc, err := ParseExternalName(out)
	if err != nil {
		t.Fatalf("roundtrip parse err: %v", err)
	}
	if origin != "sap.default" || name != "bob@example.com" || rc != "Subaccount Service Administrator" {
		t.Errorf("roundtrip mismatch: got (%q,%q,%q)", origin, name, rc)
	}
}
